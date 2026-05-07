package tools

import (
	"context"
	"fmt"

	"github.com/Quazmoz/groupme-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterLoginTool registers the authentication tool.
func RegisterLoginTool(s *server.MCPServer, store auth.TokenStore, userID string) {
	loginTool := mcp.NewTool("groupme_login",
		mcp.WithDescription("Log in to GroupMe with your access token."),
		mcp.WithString("token",
			mcp.Description("Your GroupMe Access Token (starts with 'token_')"),
		),
	)

	s.AddTool(loginTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// If userID is missing/anonymous, we can't save the token persistently
		if userID == "" || userID == "_anonymous_" {
			return mcp.NewToolResultError("Cannot log in: No persistent User ID found in request context (Are you connected via an authenticated channel?)"), nil
		}

		args := request.GetArguments()
		tokenVal, ok := args["token"]
		if !ok {
			return mcp.NewToolResultError("Token argument is required."), nil
		}
		token, ok := tokenVal.(string)
		if !ok || token == "" {
			return mcp.NewToolResultError("Token must be a non-empty string."), nil
		}

		// Register the token
		if err := store.Register(userID, token); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to register token: %v", err)), nil
		}

		return mcp.NewToolResultText("Successfully logged in! You can now use GroupMe tools."), nil
	})
}
