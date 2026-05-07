package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/Quazmoz/groupme-mcp/internal/client"
)

// Handlers provides HTTP handlers for authentication endpoints.
type Handlers struct {
	store     TokenStore
	jwtSecret string
	audience  string
	logger    *slog.Logger
}

// ClientMiddleware returns a middleware that injects the GroupMe client into the context
// based on: 1) X-GroupMe-Access-Token, 2) X-Authenticated-User (Context Forge), or 3) JWT
func (h *Handlers) ClientMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check for direct GroupMe token (Stateless/UserValves mode)
		if os.Getenv("DIRECT_TOKEN_HEADER_ENABLED") == "true" {
			if directToken := r.Header.Get("X-GroupMe-Access-Token"); directToken != "" {
				c := client.New(directToken, h.logger)
				ctx := client.NewContext(r.Context(), c)
				h.logger.Info("Middleware: Injected client from X-GroupMe-Access-Token header")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// 2. Resolve User ID (Context Forge / Python Tool / JWT)
		userID := h.getUserID(r)
		if userID != "" {
			h.logger.Info("Middleware: Resolved user identity", "user_id", userID)

			groupmeToken, err := h.store.GetDecrypted(userID)
			if err != nil {
				h.logger.Warn("Middleware: No GroupMe token found for user", "user_id", userID)
				// Continue without client - will fall back to global if configured
				next.ServeHTTP(w, r)
				return
			}

			c := client.New(groupmeToken, h.logger)
			ctx := client.NewContext(r.Context(), c)
			h.logger.Info("Middleware: Injected authenticated client", "user_id", userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

	})
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(store TokenStore, jwtSecret, audience string, logger *slog.Logger) *Handlers {
	return &Handlers{
		store:     store,
		jwtSecret: jwtSecret,
		audience:  audience,
		logger:    logger,
	}
}

// ErrorResponse represents a JSON error response.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// RegisterRequest is the request body for /auth/register.
type RegisterRequest struct {
	GroupMeToken string `json:"groupme_token"`
}

// RegisterResponse is the response for /auth/register.
type RegisterResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// StatusResponse is the response for /auth/status.
type StatusResponse struct {
	Connected bool   `json:"connected"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// RevokeResponse is the response for /auth/revoke.
type RevokeResponse struct {
	Status string `json:"status"`
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, message string, code int) {
	writeJSON(w, code, ErrorResponse{Error: message, Code: code})
}

// validateRequest extracts and validates the JWT, returning the user ID.
func (h *Handlers) validateRequest(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	return ValidateOpenWebUIJWT(authHeader, h.jwtSecret, h.audience)
}

// getUserID tries multiple auth methods: JWT, X-Authenticated-User, X-User-Email
func (h *Handlers) getUserID(r *http.Request) string {
	// Method 1: Try JWT validation
	userID, err := h.validateRequest(r)
	if err == nil && userID != "" {
		return userID
	}
	// Method 2: Check for X-Authenticated-User header (Context Forge)
	if os.Getenv("TRUSTED_PROXY_HEADERS_ENABLED") == "true" {
		if userID = r.Header.Get("X-Authenticated-User"); userID != "" {
			return userID
		}
		// Method 3: Check for X-User-Email header (Python Tool)
		return r.Header.Get("X-User-Email")
	}
	return ""
}

// HandleRegister handles POST /auth/register.
// Registers a GroupMe token for the authenticated user.
// Supports: JWT, X-Authenticated-User header, or X-User-Email header.
func (h *Handlers) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user identity from JWT or headers
	userID := h.getUserID(r)
	if userID == "" {
		h.logger.Warn("No user identification provided for registration")
		writeError(w, "unauthorized - provide JWT, X-Authenticated-User, or X-User-Email header", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Register the token
	if err := h.store.Register(userID, req.GroupMeToken); err != nil {
		if err == ErrInvalidGroupMeToken {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.logger.Error("Failed to register token", "error", err, "user_id", userID)
		writeError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, RegisterResponse{
		Status:  "ok",
		Message: "GroupMe token registered",
	})
}

// HandleStatus handles GET /auth/status.
// Returns whether the authenticated user has a registered token.
func (h *Handlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user identity from JWT or headers
	userID := h.getUserID(r)
	if userID == "" {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Check token status
	token, exists := h.store.Status(userID)
	if !exists {
		writeJSON(w, http.StatusOK, StatusResponse{Connected: false})
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{
		Connected: true,
		ExpiresAt: token.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// HandleRevoke handles POST /auth/revoke.
// Removes the GroupMe token for the authenticated user.
func (h *Handlers) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user identity from JWT or headers
	userID := h.getUserID(r)
	if userID == "" {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Revoke the token
	h.store.Revoke(userID)

	writeJSON(w, http.StatusOK, RevokeResponse{Status: "revoked"})
}
