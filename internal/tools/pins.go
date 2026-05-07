package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterPinTools registers pin-related MCP tools.
func RegisterPinTools(s *server.MCPServer, c *client.Client) {
	// List pinned messages tool
	listPinsTool := mcp.NewTool("groupme_list_pinned_messages",
		mcp.WithDescription("List pinned messages in a group."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the group (e.g. '12345678'). DO NOT use the group name."),
		),
	)
	s.AddTool(listPinsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		groupID := getFirstStringArg(args, "group_id", "conversation_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		messages, err := c.ListPinnedMessages(ctx, groupID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list pinned messages: %v", err)), nil
		}

		if len(messages) == 0 {
			return mcp.NewToolResultText("No pinned messages found in this group."), nil
		}

		result, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Pin message tool
	pinMessageTool := mcp.NewTool("groupme_pin_message",
		mcp.WithDescription("Pin a message in a group."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the group (e.g. '12345678'). DO NOT use the group name."),
		),
		mcp.WithString("message_id",
			mcp.Required(),
			mcp.Description("The ID of the message to pin"),
		),
	)
	s.AddTool(pinMessageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		groupID := getFirstStringArg(args, "group_id", "conversation_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}
		messageID := getFirstStringArg(args, "message_id", "id")
		if messageID == "" {
			return mcp.NewToolResultError("message_id is required"), nil
		}

		if err := c.PinMessage(ctx, groupID, messageID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to pin message: %v", err)), nil
		}

		return mcp.NewToolResultText("Message pinned successfully"), nil
	})

	// Unpin message tool
	unpinMessageTool := mcp.NewTool("groupme_unpin_message",
		mcp.WithDescription("Unpin a message in a group."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the group (e.g. '12345678'). DO NOT use the group name."),
		),
		mcp.WithString("message_id",
			mcp.Required(),
			mcp.Description("The ID of the message to unpin"),
		),
	)
	s.AddTool(unpinMessageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		groupID := getFirstStringArg(args, "group_id", "conversation_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}
		messageID := getFirstStringArg(args, "message_id", "id")
		if messageID == "" {
			return mcp.NewToolResultError("message_id is required"), nil
		}

		if err := c.UnpinMessage(ctx, groupID, messageID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to unpin message: %v", err)), nil
		}

		return mcp.NewToolResultText("Message unpinned successfully"), nil
	})
}
