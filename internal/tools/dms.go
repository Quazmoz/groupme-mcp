package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterDMTools registers direct message tools.
func RegisterDMTools(s *server.MCPServer, c *client.Client) {
	// List chats tool
	listChatsTool := mcp.NewTool("groupme_list_chats",
		mcp.WithDescription("List your DM conversations. Returns users you've chatted with."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithNumber("page",
			mcp.Description("Page number (default 1)."),
		),
		mcp.WithNumber("per_page",
			mcp.Description("Chats per page (default 20)."),
		),
	)
	s.AddTool(listChatsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		page := 1
		if p, ok := getArgs(request)["page"].(float64); ok {
			page = int(p)
		}
		perPage := 20
		if pp, ok := getArgs(request)["per_page"].(float64); ok {
			perPage = int(pp)
		}

		chats, err := c.ListChats(ctx, page, perPage)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list chats: %v", err)), nil
		}

		result, err := json.MarshalIndent(chats, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// List DM messages tool
	listDMsTool := mcp.NewTool("groupme_list_dm_messages",
		mcp.WithDescription("Read messages from a DM by user ID. Use groupme_get_dm_by_name if you only know the name."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("other_user_id",
			mcp.Required(),
			mcp.Description("The User ID of the contact. Get this from 'groupme_list_chats'."),
		),
		mcp.WithString("before_id",
			mcp.Description("Pagination: Get messages older than this message ID."),
		),
	)
	s.AddTool(listDMsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		otherUserID, ok := getArgs(request)["other_user_id"].(string)
		if !ok || otherUserID == "" {
			return mcp.NewToolResultError("other_user_id is required"), nil
		}

		beforeID := ""
		if b, ok := getArgs(request)["before_id"].(string); ok {
			beforeID = b
		}

		messages, err := c.ListDirectMessages(ctx, otherUserID, beforeID, "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list DMs: %v", err)), nil
		}

		result, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Send DM tool
	sendDMTool := mcp.NewTool("groupme_send_dm",
		mcp.WithDescription("Send a DM by user ID. Use groupme_send_dm_by_name if you only know the name."),
		mcp.WithString("recipient_id",
			mcp.Required(),
			mcp.Description("The user ID of the recipient. Get this from groupme_list_chats."),
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The message text to send"),
		),
	)
	s.AddTool(sendDMTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		recipientID, ok := getArgs(request)["recipient_id"].(string)
		if !ok || recipientID == "" {
			return mcp.NewToolResultError("recipient_id is required"), nil
		}

		text, ok := getArgs(request)["text"].(string)
		if !ok || text == "" {
			return mcp.NewToolResultError("text is required"), nil
		}

		sourceGUID := uuid.New().String()
		message, err := c.SendDirectMessage(ctx, recipientID, text, sourceGUID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to send DM: %v", err)), nil
		}

		result, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// COMBO TOOL: Get DM messages by person's name
	getDMByNameTool := mcp.NewTool("groupme_get_dm_by_name",
		mcp.WithDescription("Read DM messages with a person by name."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the person"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of messages to retrieve (default 20)"),
		),
	)
	s.AddTool(getDMByNameTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		name, ok := getArgs(request)["name"].(string)
		if !ok || name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}

		// Find the chat by name
		chat, err := c.SearchChatByName(ctx, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find DM with '%s': %v", name, err)), nil
		}

		// Get messages
		messages, err := c.ListDirectMessages(ctx, chat.OtherUser.ID, "", "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Found chat with %s but failed to get messages: %v", chat.OtherUser.Name, err)), nil
		}

		response := struct {
			OtherUser struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"other_user"`
			Messages []client.DirectMessage `json:"messages"`
		}{
			OtherUser: struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{ID: chat.OtherUser.ID, Name: chat.OtherUser.Name},
			Messages: messages,
		}

		result, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// COMBO TOOL: Send DM by person's name
	sendDMByNameTool := mcp.NewTool("groupme_send_dm_by_name",
		mcp.WithDescription("Send a DM to someone by name. Resolves name to user ID automatically."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the recipient"),
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The message text to send"),
		),
	)
	s.AddTool(sendDMByNameTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		name, ok := getArgs(request)["name"].(string)
		if !ok || name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}

		text, ok := getArgs(request)["text"].(string)
		if !ok || text == "" {
			return mcp.NewToolResultError("text is required"), nil
		}

		// Find the chat by name
		chat, err := c.SearchChatByName(ctx, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find DM with '%s': %v", name, err)), nil
		}

		// Send message
		sourceGUID := uuid.New().String()
		message, err := c.SendDirectMessage(ctx, chat.OtherUser.ID, text, sourceGUID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Found %s but failed to send message: %v", chat.OtherUser.Name, err)), nil
		}

		result, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Message sent to %s!\n%s", chat.OtherUser.Name, string(result))), nil
	})
}
