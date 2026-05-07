package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterBlockTools registers user blocking tools.
func RegisterBlockTools(s *server.MCPServer, c *client.Client) {
	// List blocks tool
	listBlocksTool := mcp.NewTool("groupme_list_blocks",
		mcp.WithDescription("List users you have blocked."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("user_id",
			mcp.Required(),
			mcp.Description("Your User ID (get from groupme_get_current_user)."),
		),
	)
	s.AddTool(listBlocksTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		userID, ok := getArgs(request)["user_id"].(string)
		if !ok || userID == "" {
			return mcp.NewToolResultError("user_id is required"), nil
		}

		blocks, err := c.ListBlocks(ctx, userID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list blocks: %v", err)), nil
		}

		result, err := json.MarshalIndent(blocks, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Block user tool
	blockUserTool := mcp.NewTool("groupme_block_user",
		mcp.WithDescription("Block a user from messaging you."),
		mcp.WithString("user_id",
			mcp.Required(),
			mcp.Description("Your User ID."),
		),
		mcp.WithString("other_user_id",
			mcp.Required(),
			mcp.Description("The User ID of the person to block."),
		),
	)
	s.AddTool(blockUserTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		userID, ok := getArgs(request)["user_id"].(string)
		if !ok || userID == "" {
			return mcp.NewToolResultError("user_id is required"), nil
		}

		otherUserID, ok := getArgs(request)["other_user_id"].(string)
		if !ok || otherUserID == "" {
			return mcp.NewToolResultError("other_user_id is required"), nil
		}

		block, err := c.BlockUser(ctx, userID, otherUserID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to block user: %v", err)), nil
		}

		result, err := json.MarshalIndent(block, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Unblock user tool
	unblockUserTool := mcp.NewTool("groupme_unblock_user",
		mcp.WithDescription("Unblock a user."),
		mcp.WithString("user_id",
			mcp.Required(),
			mcp.Description("Your own user ID"),
		),
		mcp.WithString("other_user_id",
			mcp.Required(),
			mcp.Description("The user ID of the person to unblock"),
		),
	)
	s.AddTool(unblockUserTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		userID, ok := getArgs(request)["user_id"].(string)
		if !ok || userID == "" {
			return mcp.NewToolResultError("user_id is required"), nil
		}

		otherUserID, ok := getArgs(request)["other_user_id"].(string)
		if !ok || otherUserID == "" {
			return mcp.NewToolResultError("other_user_id is required"), nil
		}

		if err := c.UnblockUser(ctx, userID, otherUserID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to unblock user: %v", err)), nil
		}

		return mcp.NewToolResultText("User unblocked successfully"), nil
	})

	// Check if block exists tool
	blockExistsTool := mcp.NewTool("groupme_block_exists",
		mcp.WithDescription("Check if a block exists between you and another user."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("user_id",
			mcp.Required(),
			mcp.Description("Your own user ID (get from groupme_get_current_user)"),
		),
		mcp.WithString("other_user_id",
			mcp.Required(),
			mcp.Description("The user ID to check"),
		),
	)
	s.AddTool(blockExistsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		userID, ok := getArgs(request)["user_id"].(string)
		if !ok || userID == "" {
			return mcp.NewToolResultError("user_id is required"), nil
		}

		otherUserID, ok := getArgs(request)["other_user_id"].(string)
		if !ok || otherUserID == "" {
			return mcp.NewToolResultError("other_user_id is required"), nil
		}

		exists, err := c.BlockExists(ctx, userID, otherUserID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to check block: %v", err)), nil
		}

		if exists {
			return mcp.NewToolResultText("Yes, a block exists between these users"), nil
		}
		return mcp.NewToolResultText("No block exists between these users"), nil
	})
}
