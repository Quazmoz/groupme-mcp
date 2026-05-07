package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterReactionTools registers reaction-related MCP tools.
func RegisterReactionTools(s *server.MCPServer, c *client.Client) {
	// React to message tool
	reactTool := mcp.NewTool("groupme_react_to_message",
		mcp.WithDescription("React to a message with a unicode emoji."),
		mcp.WithString("conversation_id",
			mcp.Required(),
			mcp.Description("The ID of the conversation (group ID or DM ID)"),
		),
		mcp.WithString("message_id",
			mcp.Required(),
			mcp.Description("The ID of the message to react to"),
		),
		mcp.WithString("emoji",
			mcp.Required(),
			mcp.Description("The unicode emoji character to use (e.g. '👍', '🔥'). Do not use :shortcodes:."),
		),
	)
	s.AddTool(reactTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		conversationID := getFirstStringArg(args, "conversation_id", "channel_id", "group_id")
		if conversationID == "" {
			return mcp.NewToolResultError("conversation_id is required (group_id/channel_id also accepted)"), nil
		}

		messageID := getFirstStringArg(args, "message_id", "id")
		if messageID == "" {
			return mcp.NewToolResultError("message_id is required"), nil
		}

		emoji := getFirstStringArg(args, "emoji")
		if emoji == "" {
			return mcp.NewToolResultError("emoji is required"), nil
		}

		if err := c.ReactToMessage(ctx, conversationID, messageID, emoji); err != nil {
			// Check if it's a 400 error which usually means invalid emoji
			if strings.Contains(err.Error(), "400") {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to react: The emoji '%s' might not be supported by GroupMe or the message is too old.", emoji)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("Failed to react to message: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully reacted with %s", emoji)), nil
	})
}
