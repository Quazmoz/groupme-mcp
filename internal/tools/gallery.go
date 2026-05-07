package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterGalleryTools registers gallery-related MCP tools.
func RegisterGalleryTools(s *server.MCPServer, c *client.Client) {
	// List gallery tool
	listGalleryTool := mcp.NewTool("groupme_list_gallery",
		mcp.WithDescription("Browse the media gallery of a group or DM. You can provide conversation_id directly, or pass group_id/group_name for automatic resolution."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("conversation_id",
			mcp.Description("The numeric ID of the conversation (e.g. '12345678'). Optional if group_id or group_name is provided."),
		),
		mcp.WithString("group_id",
			mcp.Description("Fallback alias for conversation_id when browsing a group gallery."),
		),
		mcp.WithString("group_name",
			mcp.Description("Optional group name for auto-resolution when IDs are unknown. Must match exactly if multiple groups share similar names."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of gallery items to retrieve (default 20, max 100)"),
		),
		mcp.WithString("before_id",
			mcp.Description("Get items before this message ID (for pagination)"),
		),
		mcp.WithBoolean("accept_files",
			mcp.Description("If true, includes non-image files in the results. Default is false (images only)."),
		),
	)
	s.AddTool(listGalleryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		conversationID := getFirstStringArg(args, "conversation_id", "channel_id", "group_id")

		if conversationID == "" {
			groupName := getFirstStringArg(args, "group_name")
			if groupName != "" {
				groups, err := c.ListAllGroupsWithOptions(ctx, true)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve group_name: %v", err)), nil
				}

				matches := make([]client.Group, 0)
				for _, g := range groups {
					if strings.EqualFold(strings.TrimSpace(g.Name), groupName) {
						matches = append(matches, g)
					}
				}

				switch len(matches) {
				case 1:
					conversationID = matches[0].ID
				case 0:
					return mcp.NewToolResultError(fmt.Sprintf("No group found with name %q. Provide conversation_id directly or use groupme_list_groups to discover IDs.", groupName)), nil
				default:
					return mcp.NewToolResultError(fmt.Sprintf("Multiple groups matched %q. Provide conversation_id or group_id explicitly.", groupName)), nil
				}
			}
		}

		if conversationID == "" {
			return mcp.NewToolResultError("conversation_id is required (or provide group_id/group_name)"), nil
		}

		limit := getIntArg(args, "limit", 20, 1, 100)

		beforeID := getFirstStringArg(args, "before_id", "before")

		acceptFiles := getBoolArg(args, "accept_files", false)

		messages, err := c.ListGallery(ctx, conversationID, limit, beforeID, acceptFiles)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list gallery: %v", err)), nil
		}

		// Filter result for conciseness - show attachments primarily
		type GalleryItem struct {
			MessageID   string              `json:"message_id"`
			Sender      string              `json:"sender"`
			CreatedAt   int64               `json:"created_at"`
			Text        string              `json:"text,omitempty"`
			Attachments []client.Attachment `json:"attachments"`
		}

		var items []GalleryItem
		for _, msg := range messages {
			items = append(items, GalleryItem{
				MessageID:   msg.ID,
				Sender:      msg.Name,
				CreatedAt:   msg.CreatedAt,
				Text:        msg.Text,
				Attachments: msg.Attachments,
			})
		}

		result, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}
