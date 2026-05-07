// Package main is the entry point for the GroupMe MCP Server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Quazmoz/groupme-mcp/internal/auth"
	"github.com/Quazmoz/groupme-mcp/internal/client"
	intserver "github.com/Quazmoz/groupme-mcp/internal/server"
	"github.com/Quazmoz/groupme-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// getEnvOrDefault returns the environment variable value or the default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault returns the environment variable as int or the default if not set.
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func resolveLogLevel() slog.Level {
	if debugValue := strings.TrimSpace(os.Getenv("DEBUG")); debugValue != "" {
		if debugEnabled, err := strconv.ParseBool(debugValue); err == nil && debugEnabled {
			return slog.LevelDebug
		}
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// registerToolsByProfile registers tools on the MCP server based on the profile.
func registerToolsByProfile(s *server.MCPServer, c *client.Client, profile string) {
	tools.RegisterMetaTools(s, c)

	switch profile {
	case "core":
		tools.RegisterCoreTools(s, c)
	case "messaging":
		tools.RegisterMessagingTools(s, c)
	default: // "all"
		tools.RegisterGroupTools(s, c)
		tools.RegisterMessageTools(s, c)
		tools.RegisterBotTools(s, c)
		tools.RegisterUserTools(s, c)
		tools.RegisterDMTools(s, c)
		tools.RegisterBlockTools(s, c)
		tools.RegisterPollTools(s, c)
		tools.RegisterCalendarTools(s, c)
		tools.RegisterPinTools(s, c)
		tools.RegisterUploadTools(s, c)
		tools.RegisterReactionTools(s, c)
		tools.RegisterGalleryTools(s, c)
	}
}

// getToolRegistrars returns the tool registrar functions based on the profile.
func getToolRegistrars(profile string) []intserver.ToolRegistrar {
	switch profile {
	case "core":
		return []intserver.ToolRegistrar{
			tools.RegisterMetaTools,
			tools.RegisterCoreTools,
		}
	case "messaging":
		return []intserver.ToolRegistrar{
			tools.RegisterMetaTools,
			tools.RegisterMessagingTools,
		}
	default: // "all"
		return []intserver.ToolRegistrar{
			tools.RegisterMetaTools,
			tools.RegisterGroupTools,
			tools.RegisterMessageTools,
			tools.RegisterBotTools,
			tools.RegisterUserTools,
			tools.RegisterDMTools,
			tools.RegisterBlockTools,
			tools.RegisterPollTools,
			tools.RegisterCalendarTools,
			tools.RegisterPinTools,
			tools.RegisterUploadTools,
			tools.RegisterReactionTools,
			tools.RegisterGalleryTools,
		}
	}
}

func main() {
	// Initialize structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: resolveLogLevel()}))

	// Auth configuration
	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	jwtSecret := os.Getenv("JWT_SECRET")
	mcpServerURI := getEnvOrDefault("MCP_SERVER_URI", "mcp://groupme.internal")
	tokenExpiryDays := getEnvIntOrDefault("TOKEN_EXPIRY_DAYS", 90)
	defaultUser := os.Getenv("DEFAULT_USER") // Fallback user when no auth provided

	// Check if we're in multi-user mode (auth enabled)
	multiUserMode := encryptionKey != "" && jwtSecret != ""

	var token string
	if multiUserMode {
		if len(encryptionKey) < 32 {
			logger.Error("ENCRYPTION_KEY must be at least 32 characters long for secure AES-256 encryption")
			os.Exit(1)
		}
	} else {
		// Single user mode requires token if auth is disabled
		token = os.Getenv("GROUPME_ACCESS_TOKEN")
		if token == "" {
			logger.Error("GROUPME_ACCESS_TOKEN environment variable is required (or enable multi-user mode with ENCRYPTION_KEY and JWT_SECRET)")
			os.Exit(1)
		}
	}

	// Create GroupMe API client (global fallback)
	groupmeClient := client.New(token, logger)

	// Tool profile: controls which tools are registered.
	// "all" (default) = all 72 tools, "core" = ~30 essential tools, "messaging" = messaging-focused subset.
	toolProfile := getEnvOrDefault("TOOL_PROFILE", "all")

	// Create MCP server
	s := server.NewMCPServer(
		"GroupMe MCP Server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	// Register tools based on profile
	registerToolsByProfile(s, groupmeClient, toolProfile)

	// Add instructions resource (model-guidance that was previously crammed into tool descriptions)
	instructionsResource := mcp.NewResource(
		"instructions://groupme",
		"GroupMe MCP Usage Guide",
		mcp.WithResourceDescription("How to use GroupMe MCP tools effectively"),
		mcp.WithMIMEType("text/plain"),
	)
	s.AddResource(instructionsResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "instructions://groupme",
				MIMEType: "text/plain",
				Text: `GroupMe MCP Tool Guide:
- IMPORTANT: Before saying you cannot access GroupMe tools, call groupme_list_available_tools and groupme_get_server_capabilities.
- If groupme_get_server_capabilities.auth_verified is false, explain the auth issue and how to fix it (groupme_login or token configuration).
- For prompts like "list past N messages from <group>", call groupme_get_group_messages with group_name and limit.
- For file/document requests, do NOT use groupme_send_message. Use groupme_upload_file (path or file_data_base64 + filename).
- For requests like "like all messages from <user> in this group": first fetch messages for the target group, then filter by sender name, then call groupme_like_message for each message_id.
- Tools ending in '_by_name' accept group/user names directly and are preferred when you only know a name.
- Base tools (groupme_list_messages, groupme_send_message, etc.) require numeric IDs.
- Use groupme_list_groups to discover group IDs, or use _by_name variants to skip the lookup.
- For subtopics/channels: first find the parent group, then use groupme_get_group_subtopics.
- Subgroups/subtopics are accessed under the groups namespace: list them with /groups/{parent_group_id}/subgroups, then read their messages with /groups/{subgroup_id}/messages.
- Do NOT use /conversations/{id}/messages for subgroup/subtopic message retrieval.
- If GroupMe behavior is undocumented or inconsistent, use groupme_probe_api_endpoints to compare read-only endpoint candidates before concluding the API is unsupported.
- Example undocumented-endpoint probe: call groupme_probe_api_endpoints with candidates_json like [{"label":"subgroups-list","endpoint":"/groups/90951330/subgroups?page=1&per_page=25"},{"label":"subgroup-messages","endpoint":"/groups/109458263/messages?limit=5"},{"label":"baseline-groups","endpoint":"/groups?page=1&per_page=5"}].
- group_id must always be a numeric ID, never a group name.
- membership_id (used in leave/remove) is different from user_id — get it from the members array in groupme_get_group.`,
			},
		}, nil
	})

	// Add health check resource
	healthResource := mcp.NewResource(
		"health://status",
		"Health Status",
		mcp.WithResourceDescription("Server health status"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(healthResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "health://status",
				MIMEType: "application/json",
				Text:     `{"status": "healthy", "service": "groupme-mcp-server"}`,
			},
		}, nil
	})

	// Get port from environment or default
	port := getEnvOrDefault("PORT", "5000")
	transport := getEnvOrDefault("MCP_TRANSPORT", "http")

	if transport == "stdio" {
		// Use stdio transport (for direct integration)
		logger.Info("Starting GroupMe MCP Server with stdio transport")
		if err := server.ServeStdio(s); err != nil {
			logger.Error("Server error", "error", err)
			os.Exit(1)
		}
	} else {

		// Create router
		mux := http.NewServeMux()

		// Register health check
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "healthy", "service": "groupme-mcp-server"}`))
		})

		// Create tool executor for REST API
		toolExecutor := tools.NewToolExecutor(groupmeClient, logger)

		// Register auth endpoints if multi-user mode is enabled
		if multiUserMode {
			var tokenStore auth.TokenStore
			var err error

			// Priority: 1) PostgreSQL (DATABASE_URL), 2) Redis, 3) In-memory
			databaseURL := os.Getenv("DATABASE_URL")
			redisHost := os.Getenv("REDIS_HOST")

			if databaseURL != "" {
				// Use PostgreSQL store (preferred)
				logger.Info("Initializing PostgreSQL token store")
				tokenStore, err = auth.NewPostgresStore(databaseURL, encryptionKey, tokenExpiryDays, logger)
				if err != nil {
					logger.Error("Failed to initialize PostgreSQL store", "error", err)
					os.Exit(1)
				}
			} else if redisHost != "" {
				// Use Redis store (legacy)
				redisPort := getEnvOrDefault("REDIS_PORT", "6379")
				redisPassword := os.Getenv("REDIS_PASSWORD")
				redisDB := getEnvIntOrDefault("REDIS_DB", 0)
				redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

				logger.Info("Initializing Redis token store", "addr", redisAddr, "db", redisDB)
				tokenStore, err = auth.NewRedisStore(redisAddr, redisPassword, redisDB, encryptionKey, tokenExpiryDays, logger)
				if err != nil {
					logger.Error("Failed to initialize Redis store", "error", err)
					os.Exit(1)
				}
			} else {
				// Use Memory store (no persistence - development only)
				logger.Warn("Initializing in-memory token store (no persistence)")
				tokenStore = auth.NewMemoryStore(encryptionKey, tokenExpiryDays, logger)
			}

			// Create auth handlers
			authHandlers := auth.NewHandlers(tokenStore, jwtSecret, mcpServerURI, logger)

			// Register auth endpoints
			mux.HandleFunc("/auth/register", authHandlers.HandleRegister)
			mux.HandleFunc("/auth/status", authHandlers.HandleStatus)
			mux.HandleFunc("/auth/revoke", authHandlers.HandleRevoke)

			// Wrap tool/execute with auth middleware for Context Forge support
			mux.Handle("/tool/execute", authHandlers.ClientMiddleware(http.HandlerFunc(toolExecutor.HandleToolExecute)))

			// Create tool registrars for StatelessMCPHandler (respects TOOL_PROFILE)
			toolRegistrars := getToolRegistrars(toolProfile)

			// Create StatelessMCPHandler with per-user authentication
			// This creates a fresh MCPServer per-user with their authenticated client
			statelessMCPHandler := intserver.NewStatelessMCPHandler(
				logger,
				tokenStore,
				tokenStore.GetDecrypted, // Token getter function
				toolRegistrars,
				tools.RegisterLoginTool, // Register login tool
				groupmeClient,           // Fallback for anonymous requests
				jwtSecret,               // For decoding Authorization header
				mcpServerURI,            // Expected audience in JWT
				defaultUser,             // Fallback user when no auth provided
			)

			// Register /mcp endpoint (no auth middleware - handler does its own auth)
			mux.Handle("/mcp", statelessMCPHandler)
			// Also register /message for the JSON-RPC channel (MCP default)
			mux.Handle("/message", statelessMCPHandler)
		} else {
			// Non-multi-user mode - use simple StatelessMCPHandler without token lookup
			toolRegistrars := getToolRegistrars(toolProfile)

			statelessMCPHandler := intserver.NewStatelessMCPHandler(
				logger,
				nil, // No token store in single-user mode
				nil, // No token getter in single-user mode
				toolRegistrars,
				nil, // No login tool in single-user mode
				groupmeClient,
				"", // No JWT in single-user mode
				"", // No audience check
				"", // No default user needed
			)

			// Register handlers without auth wrapper
			mux.HandleFunc("/tool/execute", toolExecutor.HandleToolExecute)
			mux.Handle("/mcp", statelessMCPHandler)
		}

		// Register SSE endpoint (legacy/backwards compatibility)
		// mux.Handle("/sse", sseServer.SSEHandler())

		// Add CORS middleware
		allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
		corsHandler := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				origin := r.Header.Get("Origin")
				if origin != "" {
					allowed := false
					if allowedOrigins == "*" {
						allowed = true
					} else if allowedOrigins != "" {
						for _, o := range strings.Split(allowedOrigins, ",") {
							if strings.TrimSpace(o) == origin {
								allowed = true
								break
							}
						}
					} else if origin == "http://localhost" || strings.HasPrefix(origin, "http://localhost:") {
						allowed = true
					}

					if allowed {
						w.Header().Set("Access-Control-Allow-Origin", origin)
					}
				}

				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-GroupMe-Access-Token, X-Authenticated-User, X-User-Email")

				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}

				h.ServeHTTP(w, r)
			})
		}

		// Create server with timeouts
		addr := fmt.Sprintf(":%s", port)
		srv := &http.Server{
			Addr:         addr,
			Handler:      corsHandler(mux),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		// Graceful shutdown handling
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			sig := <-sigChan
			logger.Info("Received shutdown signal", "signal", sig)

			// Give outstanding requests 30 seconds to complete
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := srv.Shutdown(ctx); err != nil {
				logger.Error("Graceful shutdown failed", "error", err)
				os.Exit(1)
			}
			logger.Info("Server shutdown complete")
		}()

		// Start server
		logger.Info("MCP Server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "error", err)
			os.Exit(1)
		}
	}
}
