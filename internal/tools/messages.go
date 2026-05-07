package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterMessageTools registers message-related MCP tools.
func RegisterMessageTools(s *server.MCPServer, c *client.Client) {
	// List messages tool
	listMessagesTool := mcp.NewTool("groupme_list_messages",
		mcp.WithDescription("Read messages from a group by numeric ID. Use groupme_get_group_messages if you only know the name."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the group to post to. DO NOT use the group name."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of messages to retrieve (default 20, max 100). Increase this if you need more context."),
		),
		mcp.WithString("before_id",
			mcp.Description("Pagination: Get messages created BEFORE this message ID. Use this to scroll back in history."),
		),
	)
	s.AddTool(listMessagesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		groupID := getFirstStringArg(args, "group_id", "conversation_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		limit := getIntArg(args, "limit", 20, 1, 100)

		beforeID := getFirstStringArg(args, "before_id", "before")

		messages, err := c.ListMessages(ctx, groupID, limit, beforeID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list messages: %v", err)), nil
		}

		result, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	listSubtopicMessagesTool := mcp.NewTool("groupme_list_subtopic_messages",
		mcp.WithDescription("Read messages from a subtopic/channel by subgroup_id or subtopic_name. Subgroups are listed under /groups/{parent_group_id}/subgroups, then read through /groups/{subgroup_id}/messages."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Description("Optional numeric ID of the parent group. Use this to list/resolve subtopics under /groups/{parent_group_id}/subgroups. It is not the message-read path once you already know subgroup_id."),
		),
		mcp.WithString("subgroup_id",
			mcp.Description("The numeric ID of the subtopic/channel. This ID behaves like a group ID for message reads and is fetched via /groups/{subgroup_id}/messages."),
		),
		mcp.WithString("subtopic_name",
			mcp.Description("Optional subtopic name to resolve, such as 'Online Mission'."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of messages to retrieve (default 20, max 100)."),
		),
		mcp.WithString("before_id",
			mcp.Description("Pagination cursor for older subgroup messages."),
		),
	)
	s.AddTool(listSubtopicMessagesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		result, err := listSubtopicMessagesHandler(ctx, c, getArgs(request))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list subtopic messages: %v", err)), nil
		}

		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})

	getLatestYouTubeLinkTool := mcp.NewTool("groupme_get_latest_youtube_link_from_subtopic",
		mcp.WithDescription("Find the latest YouTube link in a subtopic/channel. Subgroup messages are read through /groups/{subgroup_id}/messages, not /conversations/{id}/messages."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Description("Optional numeric ID of the parent group used to resolve/list subtopics under /groups/{parent_group_id}/subgroups."),
		),
		mcp.WithString("subgroup_id",
			mcp.Description("The numeric ID of the subtopic/channel. This ID is used as the group-like message scope for /groups/{subgroup_id}/messages."),
		),
		mcp.WithString("subtopic_name",
			mcp.Description("Optional subtopic name to resolve, such as 'Online Mission'."),
		),
		mcp.WithNumber("lookback_hours",
			mcp.Description("Only consider messages from this many hours back. Default 24."),
		),
		mcp.WithNumber("max_messages",
			mcp.Description("How many subgroup messages to inspect. Default 100."),
		),
	)
	s.AddTool(getLatestYouTubeLinkTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		result, err := getLatestYouTubeLinkFromSubtopicHandler(ctx, c, getArgs(request))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get latest YouTube link from subtopic: %v", err)), nil
		}

		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})

	forwardLatestYouTubeLinkTool := mcp.NewTool("groupme_forward_latest_youtube_link_from_subtopic",
		mcp.WithDescription("Find the latest YouTube link in a subtopic and forward it to another group. Subtopics are listed under the parent group's /subgroups route and read via /groups/{subgroup_id}/messages."),
		mcp.WithString("source_group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the parent/source group."),
		),
		mcp.WithString("subgroup_id",
			mcp.Description("The numeric ID of the source subtopic/channel. Optional if subtopic_name is provided."),
		),
		mcp.WithString("subtopic_name",
			mcp.Description("Optional source subtopic name to resolve, such as 'Online Mission'."),
		),
		mcp.WithString("target_group_id",
			mcp.Description("Numeric ID of the destination group. Optional if target_group_name is provided."),
		),
		mcp.WithString("target_group_name",
			mcp.Description("Name of the destination group. Optional if target_group_id is provided."),
		),
		mcp.WithString("prefix_text",
			mcp.Description("Optional text to prepend before the forwarded link."),
		),
		mcp.WithNumber("lookback_hours",
			mcp.Description("Only consider source messages from this many hours back. Default 24."),
		),
		mcp.WithNumber("max_messages",
			mcp.Description("How many source subgroup messages to inspect. Default 100."),
		),
		mcp.WithBoolean("allow_low_confidence",
			mcp.Description("Override the safety check and allow forwarding even if only a fallback subgroup message endpoint succeeded."),
		),
	)
	s.AddTool(forwardLatestYouTubeLinkTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		result, err := forwardLatestYouTubeLinkFromSubtopicHandler(ctx, c, getArgs(request))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to forward latest YouTube link from subtopic: %v", err)), nil
		}

		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})

	// Send message tool
	sendMessageTool := mcp.NewTool("groupme_send_message",
		mcp.WithDescription("Send a text-only message to a group by numeric ID. For documents/videos use groupme_upload_file or groupme_upload_video."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the group to post to. DO NOT use the group name."),
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The content of the message. Max 1000 chars."),
		),
	)
	s.AddTool(sendMessageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := CheckHighImpact(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		c := client.Get(ctx, c)
		args := getArgs(request)
		groupID := getFirstStringArg(args, "group_id", "conversation_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		text := getFirstStringArg(args, "text", "message")
		if text == "" {
			return mcp.NewToolResultError("text is required"), nil
		}
		if err := CheckMessageLength(text); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := CheckDedupe(groupID, text); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Generate a unique source GUID for deduplication
		sourceGUID := uuid.New().String()

		message, err := c.SendMessage(ctx, groupID, text, sourceGUID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to send message: %v", err)), nil
		}

		result, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Like message tool
	likeMessageTool := mcp.NewTool("groupme_like_message",
		mcp.WithDescription("Like (heart) a message by conversation_id and message_id."),
		mcp.WithString("conversation_id",
			mcp.Required(),
			mcp.Description("The numeric conversation/group ID. DO NOT use the group name."),
		),
		mcp.WithString("message_id",
			mcp.Required(),
			mcp.Description("The ID of the message to like"),
		),
	)
	s.AddTool(likeMessageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		if err := c.LikeMessage(ctx, conversationID, messageID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to like message: %v", err)), nil
		}

		return mcp.NewToolResultText("Message liked successfully"), nil
	})

	// Unlike message tool
	unlikeMessageTool := mcp.NewTool("groupme_unlike_message",
		mcp.WithDescription("Remove a like (heart) from a message."),
		mcp.WithString("conversation_id",
			mcp.Required(),
			mcp.Description("The numeric conversation/group ID. DO NOT use the group name."),
		),
		mcp.WithString("message_id",
			mcp.Required(),
			mcp.Description("The ID of the message to unlike"),
		),
	)
	s.AddTool(unlikeMessageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		if err := c.UnlikeMessage(ctx, conversationID, messageID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to unlike message: %v", err)), nil
		}

		return mcp.NewToolResultText("Message unliked successfully"), nil
	})

	// Search messages tool
	searchMessagesTool := mcp.NewTool("groupme_search_messages",
		mcp.WithDescription("Search messages in a group by numeric ID. Use groupme_search_in_group_by_name if you only know the name."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric group ID. DO NOT use the group name."),
		),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The text to search for (case-insensitive)"),
		),
		mcp.WithNumber("max_messages",
			mcp.Description("Maximum number of messages to search through (default 100, max 500)"),
		),
	)
	s.AddTool(searchMessagesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		groupID := getFirstStringArg(args, "group_id", "conversation_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		query := getFirstStringArg(args, "query", "search")
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		maxMessages := getIntArg(args, "max_messages", 100, 1, 500)

		messages, err := c.SearchMessagesInGroup(ctx, groupID, query, maxMessages)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to search messages: %v", err)), nil
		}

		if len(messages) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No messages found containing '%s'", query)), nil
		}

		result, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Found %d messages containing '%s':\n%s", len(messages), query, string(result))), nil
	})

	// COMBO: Search messages in group by name
	searchInGroupByNameTool := mcp.NewTool("groupme_search_in_group_by_name",
		mcp.WithDescription("Search messages in a group by name. Resolves name to ID automatically."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group to search"),
		),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The text to search for (case-insensitive)"),
		),
		mcp.WithNumber("max_messages",
			mcp.Description("Maximum number of messages to search through (default 100)"),
		),
	)
	s.AddTool(searchInGroupByNameTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		groupName, ok := getArgs(request)["group_name"].(string)
		if !ok || groupName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
		}

		query, ok := getArgs(request)["query"].(string)
		if !ok || query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		maxMessages := 100
		if m, ok := getArgs(request)["max_messages"].(float64); ok {
			maxMessages = int(m)
			if maxMessages > 500 {
				maxMessages = 500
			}
		}

		// Find the group by name
		group, err := c.SearchGroupByName(ctx, groupName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find group '%s': %v", groupName, err)), nil
		}

		messages, err := c.SearchMessagesInGroup(ctx, group.ID, query, maxMessages)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Found group '%s' but search failed: %v", group.Name, err)), nil
		}

		if len(messages) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No messages found containing '%s' in group '%s'", query, group.Name)), nil
		}

		result, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Found %d messages containing '%s' in '%s':\n%s", len(messages), query, group.Name, string(result))), nil
	})

	// COMBO TOOL: Send message with @mention by names
	sendWithMentionTool := mcp.NewTool("groupme_send_with_mention",
		mcp.WithDescription("Send a message that @mentions a user by name in a group."),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group to send to"),
		),
		mcp.WithString("mention_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the user to @mention"),
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The message text. Use @NAME where you want the mention to appear, e.g. 'Hey @John, check this out!'"),
		),
	)
	s.AddTool(sendWithMentionTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := CheckHighImpact(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		groupName, ok := getArgs(request)["group_name"].(string)
		if !ok || groupName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
		}

		mentionName, ok := getArgs(request)["mention_name"].(string)
		if !ok || mentionName == "" {
			return mcp.NewToolResultError("mention_name is required"), nil
		}

		text, ok := getArgs(request)["text"].(string)
		if !ok || text == "" {
			return mcp.NewToolResultError("text is required"), nil
		}
		if err := CheckMessageLength(text); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := CheckDedupe(groupName, text); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Find the group by name
		group, err := c.SearchGroupByName(ctx, groupName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find group '%s': %v", groupName, err)), nil
		}

		// Get full group details with members
		fullGroup, err := c.GetGroup(ctx, group.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not get group details: %v", err)), nil
		}

		// Find the member to mention
		var memberToMention *client.Member
		mentionNameLower := strings.ToLower(mentionName)
		for i, member := range fullGroup.Members {
			if strings.Contains(strings.ToLower(member.Nickname), mentionNameLower) {
				memberToMention = &fullGroup.Members[i]
				break
			}
		}

		if memberToMention == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find member '%s' in group '%s'", mentionName, group.Name)), nil
		}

		// Format the text with the mention
		// Replace @NAME with @MemberNickname and record the position
		mentionText := "@" + memberToMention.Nickname

		// Look for @<something> pattern in text and replace first occurrence
		var finalText string
		var loci [][]int
		atIdx := strings.Index(text, "@")
		if atIdx != -1 {
			// Find where the @ placeholder ends (next space or end of string)
			endIdx := strings.Index(text[atIdx:], " ")
			if endIdx == -1 {
				endIdx = len(text) - atIdx
			}
			finalText = text[:atIdx] + mentionText + text[atIdx+endIdx:]
			loci = [][]int{{atIdx, len(mentionText)}}
		} else {
			// No @ found, prepend the mention
			finalText = mentionText + " " + text
			loci = [][]int{{0, len(mentionText)}}
		}

		// Send the message with mention
		sourceGUID := uuid.New().String()
		message, err := c.SendMessageWithMentions(ctx, group.ID, finalText, sourceGUID, []string{memberToMention.UserID}, loci)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to send message: %v", err)), nil
		}

		result, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Sent message mentioning @%s in '%s'!\n%s", memberToMention.Nickname, group.Name, string(result))), nil
	})

	// COMBO TOOL: Send message that @mentions ALL members
	sendToAllTool := mcp.NewTool("groupme_send_to_all",
		mcp.WithDescription("Send a message that @mentions ALL members in a group. Use for 'notify everyone' or '@all'."),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group"),
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The message text to send (will be prefixed with member mentions)"),
		),
	)
	s.AddTool(sendToAllTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := CheckBroadcast(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		groupName, ok := getArgs(request)["group_name"].(string)
		if !ok || groupName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
		}

		text, ok := getArgs(request)["text"].(string)
		if !ok || text == "" {
			return mcp.NewToolResultError("text is required"), nil
		}
		if err := CheckMessageLength(text); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := CheckDedupe(groupName, text); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Find the group by name
		group, err := c.SearchGroupByName(ctx, groupName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find group '%s': %v", groupName, err)), nil
		}

		// Get full group details with members
		fullGroup, err := c.GetGroup(ctx, group.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not get group details: %v", err)), nil
		}

		if len(fullGroup.Members) == 0 {
			return mcp.NewToolResultError("No members found in group"), nil
		}

		// Build @all mention: "@Name1 @Name2 @Name3 text"
		var mentionParts []string
		var userIDs []string
		var loci [][]int
		currentPos := 0

		for _, member := range fullGroup.Members {
			mentionText := "@" + member.Nickname
			mentionParts = append(mentionParts, mentionText)
			userIDs = append(userIDs, member.UserID)
			loci = append(loci, []int{currentPos, len(mentionText)})
			currentPos += len(mentionText) + 1 // +1 for space
		}

		finalText := strings.Join(mentionParts, " ") + " " + text

		// Check message length
		if len(finalText) > 1000 {
			return mcp.NewToolResultError(fmt.Sprintf("Message too long with all mentions (%d chars). Max is 1000. Try a shorter message or use groupme_send_with_mention for specific users.", len(finalText))), nil
		}

		// Send the message with all mentions
		sourceGUID := uuid.New().String()
		message, err := c.SendMessageWithMentions(ctx, group.ID, finalText, sourceGUID, userIDs, loci)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to send message: %v", err)), nil
		}

		result, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Sent message to ALL %d members in '%s'!\n%s", len(fullGroup.Members), group.Name, string(result))), nil
	})

	// COMBO TOOL: Send image to group by name (supports URL or base64)
	sendImageTool := mcp.NewTool("groupme_send_image",
		mcp.WithDescription("Send an image to a group by name. Provide image_url or image_base64."),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group"),
		),
		mcp.WithString("image_url",
			mcp.Description("URL of the image to send (recommended). The image will be fetched and uploaded to GroupMe."),
		),
		mcp.WithString("image_base64",
			mcp.Description("The image data encoded as base64 (alternative to image_url)"),
		),
		mcp.WithString("text",
			mcp.Description("Optional text message to accompany the image"),
		),
	)
	s.AddTool(sendImageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := CheckHighImpact(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		args := getArgs(request)
		groupName := getFirstStringArg(args, "group_name", "name")
		if groupName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
		}

		imageURL := getFirstStringArg(args, "image_url", "url")
		imageBase64 := getFirstStringArg(args, "image_base64", "image_data", "base64_data")
		text := getFirstStringArg(args, "text", "message")
		if err := CheckMessageLength(text); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if imageURL == "" && imageBase64 == "" {
			return mcp.NewToolResultError("Either image_url or image_base64 is required"), nil
		}

		// Find the group by name
		group, err := c.SearchGroupByName(ctx, groupName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find group '%s': %v", groupName, err)), nil
		}

		var imageData []byte
		var contentType string

		if imageURL != "" {
			// Fetch image from URL
			resp, err := http.Get(imageURL)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch image from URL: %v", err)), nil
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch image: HTTP %d", resp.StatusCode)), nil
			}

			imageData, err = io.ReadAll(resp.Body)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to read image data: %v", err)), nil
			}

			// Get content type from response
			contentType = resp.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "image/png"
			}
		} else {
			// Decode base64 image
			if idx := strings.Index(imageBase64, ","); idx != -1 {
				imageBase64 = imageBase64[idx+1:]
			}
			imageData, err = base64.StdEncoding.DecodeString(imageBase64)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to decode base64 image: %v", err)), nil
			}
			contentType = "image/png"
		}

		// Upload to GroupMe's image service
		groupmeImageURL, err := c.UploadImage(ctx, imageData, contentType)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to upload image: %v", err)), nil
		}

		// Send message with image
		sourceGUID := uuid.New().String()
		message, err := c.SendMessageWithImage(ctx, group.ID, text, groupmeImageURL, sourceGUID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Image uploaded but failed to send message: %v", err)), nil
		}

		result, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Image sent to '%s'!\n%s", group.Name, string(result))), nil
	})

	// COMBO TOOL: Send location to group by name
	sendLocationTool := mcp.NewTool("groupme_send_location",
		mcp.WithDescription("Send a location pin to a group by name."),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group"),
		),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("Latitude coordinate (e.g., 40.7128 for New York)"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("Longitude coordinate (e.g., -74.0060 for New York)"),
		),
		mcp.WithString("location_name",
			mcp.Required(),
			mcp.Description("Name of the location (e.g., 'Empire State Building')"),
		),
		mcp.WithString("text",
			mcp.Description("Optional text message to accompany the location"),
		),
	)
	s.AddTool(sendLocationTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := CheckHighImpact(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		groupName, ok := getArgs(request)["group_name"].(string)
		if !ok || groupName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
		}

		lat, ok := getArgs(request)["latitude"].(float64)
		if !ok {
			return mcp.NewToolResultError("latitude is required"), nil
		}

		lng, ok := getArgs(request)["longitude"].(float64)
		if !ok {
			return mcp.NewToolResultError("longitude is required"), nil
		}

		locationName, ok := getArgs(request)["location_name"].(string)
		if !ok || locationName == "" {
			return mcp.NewToolResultError("location_name is required"), nil
		}

		text, _ := getArgs(request)["text"].(string)
		if err := CheckMessageLength(text); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Find the group by name
		group, err := c.SearchGroupByName(ctx, groupName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find group '%s': %v", groupName, err)), nil
		}

		// Send message with location
		sourceGUID := uuid.New().String()
		message, err := c.SendMessageWithLocation(ctx, group.ID, text, sourceGUID, lat, lng, locationName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to send location: %v", err)), nil
		}

		result, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Location '%s' sent to '%s'!\n%s", locationName, group.Name, string(result))), nil
	})

	// COMBO: Wait for Reply (Live Event Waiter)
	waitForReplyTool := mcp.NewTool("groupme_wait_for_reply",
		mcp.WithDescription("Wait for new messages in a group after a given message ID. Polls until reply or timeout."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group to wait within."),
		),
		mcp.WithString("since_id",
			mcp.Required(),
			mcp.Description("The message ID of the last message you saw (e.g., the message you just sent). The tool will return only messages that arrive AFTER this ID."),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Maximum seconds to wait before giving up. Default is 30, Max is 60."),
		),
	)
	s.AddTool(waitForReplyTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		groupName, ok := getArgs(request)["group_name"].(string)
		if !ok || groupName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
		}
		sinceID, ok := getArgs(request)["since_id"].(string)
		if !ok || sinceID == "" {
			return mcp.NewToolResultError("since_id is required"), nil
		}

		timeoutSeconds := 30
		if t, ok := getArgs(request)["timeout_seconds"].(float64); ok {
			timeoutSeconds = int(t)
			if timeoutSeconds > 60 {
				timeoutSeconds = 60
			}
		}

		// Find the group by name
		group, err := c.SearchGroupByName(ctx, groupName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find group '%s': %v", groupName, err)), nil
		}

		deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
		pollInterval := 2 * time.Second

		for time.Now().Before(deadline) {
			// List messages SINCE sinceID
			messages, err := c.ListMessagesWithOptions(ctx, group.ID, 20, "", sinceID, "")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to check for messages: %v", err)), nil
			}

			if len(messages) > 0 {
				result, _ := json.MarshalIndent(messages, "", "  ")
				return mcp.NewToolResultText(fmt.Sprintf("New messages received!\n%s", string(result))), nil
			}

			// Wait before polling again
			select {
			case <-ctx.Done():
				return mcp.NewToolResultError("Context cancelled while waiting for reply"), nil
			case <-time.After(pollInterval):
				// continue loop
			}
		}

		return mcp.NewToolResultText(fmt.Sprintf("Timeout: No new messages received after %d seconds.", timeoutSeconds)), nil
	})
}
