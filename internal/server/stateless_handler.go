package server

import (
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/Quazmoz/groupme-mcp/internal/auth"
	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/server"
)

// TokenGetter is a function that retrieves a GroupMe token for a user.
type TokenGetter func(userID string) (string, error)

// ToolRegistrar is a function that registers tools on an MCP server.
type ToolRegistrar func(s *server.MCPServer, c *client.Client)

// LoginToolRegistrar is a function that registers the login tool.
type LoginToolRegistrar func(s *server.MCPServer, store auth.TokenStore, userID string)

// StatelessMCPHandler wraps MCP to support stateless clients with per-user authentication.
//
// Key design: Creates a fresh MCPServer per-user with the user's authenticated client.
// This ensures the correct GroupMe token is used for each user's tool calls.
//
// Authentication flow:
// 1. Try to extract user from Authorization header (JWT)
// 2. Fall back to X-Authenticated-User header
// 3. Fall back to X-User-Email header
// 4. Fall back to defaultUser (if configured)
// 5. If none found, use anonymous/fallback client
type StatelessMCPHandler struct {
	logger             *slog.Logger
	tokenStore         auth.TokenStore
	tokenGetter        TokenGetter
	toolRegistrars     []ToolRegistrar
	loginToolRegistrar LoginToolRegistrar
	fallbackClient     *client.Client

	// JWT configuration for decoding Authorization header
	jwtSecret   string
	jwtAudience string

	// Default user to use when no auth is provided (for single-user scenarios)
	defaultUser string

	// Per-user server cache
	servers     map[string]*server.StreamableHTTPServer
	serverMutex sync.RWMutex
}

// NewStatelessMCPHandler creates a new handler that supports per-user authentication.
func NewStatelessMCPHandler(
	logger *slog.Logger,
	tokenStore auth.TokenStore,
	tokenGetter TokenGetter,
	toolRegistrars []ToolRegistrar,
	loginToolRegistrar LoginToolRegistrar,
	fallbackClient *client.Client,
	jwtSecret string,
	jwtAudience string,
	defaultUser string,
) *StatelessMCPHandler {
	if defaultUser != "" {
		logger.Info("Default user configured for anonymous requests", "default_user", defaultUser)
	}
	return &StatelessMCPHandler{
		logger:             logger,
		tokenStore:         tokenStore,
		tokenGetter:        tokenGetter,
		toolRegistrars:     toolRegistrars,
		loginToolRegistrar: loginToolRegistrar,
		fallbackClient:     fallbackClient,
		jwtSecret:          jwtSecret,
		jwtAudience:        jwtAudience,
		defaultUser:        defaultUser,
		servers:            make(map[string]*server.StreamableHTTPServer),
	}
}

// ServeHTTP handles MCP requests with per-user authentication.
func (h *StatelessMCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check for direct Bearer token in Authorization header (Manual Virtual Server support)
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token := authHeader[7:]
		// If it doesn't look like a JWT (no dots) and is long enough, assume it's a GroupMe token
		// JWTs typically have 2 dots. GroupMe tokens are opaque strings.
		hasDots := false
		for _, c := range token {
			if c == '.' {
				hasDots = true
				break
			}
		}

		if !hasDots && len(token) > 20 {
			if os.Getenv("DIRECT_TOKEN_HEADER_ENABLED") == "true" {
				h.logger.Info("Using direct GroupMe token from Authorization header")

				// Create client with direct token
				client := client.New(token, h.logger)

				// Create temp server (no caching for direct tokens to avoid leak/complexity)
				srv := h.createMCPServer("direct_token_user", client)

				srv.ServeHTTP(w, r)
				return
			} else {
				h.logger.Warn("Blocked attempt to use direct GroupMe token in Authorization header (DIRECT_TOKEN_HEADER_ENABLED=false)")
			}
		}
	}

	// Extract user ID using multiple methods
	userID := h.extractUserID(r)

	h.logger.Info("MCP request",
		"user_id", userID,
		"method", r.Method,
		"has_auth_header", r.Header.Get("Authorization") != "",
	)

	// Get or create server for this user
	srv := h.getOrCreateServer(userID)

	// Forward to user's MCP server
	srv.ServeHTTP(w, r)
}

// createMCPServer helper to instantiate the server with common logic
func (h *StatelessMCPHandler) createMCPServer(userID string, groupmeClient *client.Client) *server.StreamableHTTPServer {
	// Create a fresh MCP server with the authenticated client
	mcpServer := server.NewMCPServer(
		"GroupMe MCP Server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	// Register all tools with the authenticated client
	for _, registrar := range h.toolRegistrars {
		registrar(mcpServer, groupmeClient)
	}

	// Register login tool (if available) - this one needs the store and userID
	if h.loginToolRegistrar != nil && h.tokenStore != nil {
		h.loginToolRegistrar(mcpServer, h.tokenStore, userID)
	}

	// Create streamable server
	return server.NewStreamableHTTPServer(mcpServer)
}

// extractUserID tries multiple authentication methods to identify the user.
func (h *StatelessMCPHandler) extractUserID(r *http.Request) string {
	// Method 1: Try JWT from Authorization header
	if authHeader := r.Header.Get("Authorization"); authHeader != "" && h.jwtSecret != "" {
		userID, err := auth.ValidateOpenWebUIJWT(authHeader, h.jwtSecret, h.jwtAudience)
		if err == nil && userID != "" {
			h.logger.Debug("Extracted user from JWT", "user_id", userID)
			return userID
		}
		h.logger.Debug("JWT validation failed", "error", err)
	}

	// Method 2: Try X-Authenticated-User header (Context Forge passthrough)
	if os.Getenv("TRUSTED_PROXY_HEADERS_ENABLED") == "true" {
		if userID := r.Header.Get("X-Authenticated-User"); userID != "" {
			h.logger.Debug("Using X-Authenticated-User header", "user_id", userID)
			return userID
		}

		// Method 3: Try X-User-Email header (Python Tool)
		if userID := r.Header.Get("X-User-Email"); userID != "" {
			h.logger.Debug("Using X-User-Email header", "user_id", userID)
			return userID
		}
	}

	// Method 4: Fall back to default user (single-user mode)
	if h.defaultUser != "" {
		h.logger.Debug("Using default user", "user_id", h.defaultUser)
		return h.defaultUser
	}

	return ""
}

// getOrCreateServer returns or creates a StreamableHTTPServer for a user.
func (h *StatelessMCPHandler) getOrCreateServer(userID string) *server.StreamableHTTPServer {
	cacheKey := userID
	if cacheKey == "" {
		cacheKey = "_anonymous_"
	}

	// Fast path: check if server exists
	h.serverMutex.RLock()
	srv, exists := h.servers[cacheKey]
	h.serverMutex.RUnlock()

	if exists {
		return srv
	}

	// Slow path: create new server
	h.serverMutex.Lock()
	defer h.serverMutex.Unlock()

	// Double-check after acquiring lock
	if srv, exists = h.servers[cacheKey]; exists {
		return srv
	}

	// Get the user's GroupMe token
	var groupmeClient *client.Client
	if userID != "" && h.tokenGetter != nil {
		token, err := h.tokenGetter(userID)
		if err != nil {
			h.logger.Warn("Failed to get token for user, using fallback", "user_id", userID, "error", err)
			groupmeClient = h.fallbackClient
		} else {
			h.logger.Info("Creating authenticated MCP server for user", "user_id", userID)
			groupmeClient = client.New(token, h.logger)
		}
	} else {
		h.logger.Debug("Using fallback client", "user_id", userID, "has_token_getter", h.tokenGetter != nil)
		groupmeClient = h.fallbackClient
	}

	// Create and cache the server
	srv = h.createMCPServer(userID, groupmeClient)
	h.servers[cacheKey] = srv

	h.logger.Info("Created new MCP server for user", "user_id", userID, "cache_key", cacheKey)
	return srv
}
