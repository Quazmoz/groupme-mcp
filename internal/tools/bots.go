package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterBotTools registers bot-related MCP tools.
func RegisterBotTools(s *server.MCPServer, c *client.Client) {
	// List bots tool
	listBotsTool := mcp.NewTool("groupme_list_bots",
		mcp.WithDescription("List all bots you created. Returns bot ID, name, and group."),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	s.AddTool(listBotsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		bots, err := c.ListBots(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list bots: %v", err)), nil
		}

		result, err := json.MarshalIndent(bots, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Create bot tool
	createBotTool := mcp.NewTool("groupme_create_bot",
		mcp.WithDescription("Create a bot in a group for automated messages."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Display name of the bot."),
		),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group to add the bot to."),
		),
		mcp.WithString("callback_url",
			mcp.Description("Optional webhook URL where the bot should receive messages."),
		),
	)
	s.AddTool(createBotTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		name, ok := getArgs(request)["name"].(string)
		if !ok || name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}

		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		callbackURL := ""
		if cb, ok := getArgs(request)["callback_url"].(string); ok {
			callbackURL = cb
		}

		bot, err := c.CreateBot(ctx, name, groupID, callbackURL)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create bot: %v", err)), nil
		}

		result, err := json.MarshalIndent(bot, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Post bot message tool
	postBotMessageTool := mcp.NewTool("groupme_post_bot_message",
		mcp.WithDescription("Post a message to a group as a bot."),
		mcp.WithString("bot_id",
			mcp.Required(),
			mcp.Description("The bot ID"),
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The message text to post (max 1000 characters)"),
		),
	)
	s.AddTool(postBotMessageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		botID, ok := getArgs(request)["bot_id"].(string)
		if !ok || botID == "" {
			return mcp.NewToolResultError("bot_id is required"), nil
		}

		text, ok := getArgs(request)["text"].(string)
		if !ok || text == "" {
			return mcp.NewToolResultError("text is required"), nil
		}

		if err := c.PostBotMessage(ctx, botID, text); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to post bot message: %v", err)), nil
		}

		return mcp.NewToolResultText("Bot message posted successfully"), nil
	})

	// Destroy bot tool
	destroyBotTool := mcp.NewTool("groupme_destroy_bot",
		mcp.WithDescription("Delete a bot."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("bot_id",
			mcp.Required(),
			mcp.Description("The bot ID to delete"),
		),
	)
	s.AddTool(destroyBotTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		botID, ok := getArgs(request)["bot_id"].(string)
		if !ok || botID == "" {
			return mcp.NewToolResultError("bot_id is required"), nil
		}

		if err := c.DestroyBot(ctx, botID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to destroy bot: %v", err)), nil
		}

		return mcp.NewToolResultText("Bot deleted successfully"), nil
	})
}
