package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterUserTools registers user-related MCP tools.
func RegisterUserTools(s *server.MCPServer, c *client.Client) {
	// Get current user tool
	getCurrentUserTool := mcp.NewTool("groupme_get_current_user",
		mcp.WithDescription("Get your user profile: ID, name, email, phone."),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	s.AddTool(getCurrentUserTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		user, err := c.GetCurrentUser(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get current user: %v", err)), nil
		}

		result, err := json.MarshalIndent(user, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Update user tool
	updateUserTool := mcp.NewTool("groupme_update_user",
		mcp.WithDescription("Update your profile: name, email, or zip code."),
		mcp.WithString("name",
			mcp.Description("New display name"),
		),
		mcp.WithString("email",
			mcp.Description("New email address"),
		),
		mcp.WithString("zip_code",
			mcp.Description("New zip code"),
		),
	)
	s.AddTool(updateUserTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		name := ""
		if n, ok := getArgs(request)["name"].(string); ok {
			name = n
		}
		email := ""
		if e, ok := getArgs(request)["email"].(string); ok {
			email = e
		}
		zipCode := ""
		if z, ok := getArgs(request)["zip_code"].(string); ok {
			zipCode = z
		}

		if name == "" && email == "" && zipCode == "" {
			return mcp.NewToolResultError("At least one of name, email, or zip_code is required"), nil
		}

		user, err := c.UpdateUser(ctx, name, email, zipCode)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update user: %v", err)), nil
		}

		result, err := json.MarshalIndent(user, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}
