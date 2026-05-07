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

// RegisterPollTools registers poll-related MCP tools.
func RegisterPollTools(s *server.MCPServer, c *client.Client) {
	// List polls tool
	listPollsTool := mcp.NewTool("groupme_list_polls",
		mcp.WithDescription("List polls in a group. Filter by active, past, or all."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group to check."),
		),
		mcp.WithString("status",
			mcp.Description("Filter polls. Options: 'active' (open for voting), 'past' (closed), or 'all' (default)."),
		),
	)
	s.AddTool(listPollsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		status := "all"
		if s, ok := getArgs(request)["status"].(string); ok {
			status = s
		}

		polls, err := c.ListPolls(ctx, groupID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list polls: %v", err)), nil
		}

		// Filter locally if API doesn't support it (undocumented behavior)
		var filteredPolls []client.Poll
		if status == "all" {
			filteredPolls = polls
		} else {
			for _, poll := range polls {
				if strings.EqualFold(poll.Status, status) {
					filteredPolls = append(filteredPolls, poll)
				}
			}
		}

		result, err := json.MarshalIndent(filteredPolls, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Create poll tool
	createPollTool := mcp.NewTool("groupme_create_poll",
		mcp.WithDescription("Create a poll with a question and options (min 2)."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group."),
		),
		mcp.WithString("subject",
			mcp.Required(),
			mcp.Description("The question to ask (e.g., 'Where for lunch?')."),
		),
		mcp.WithString("options",
			mcp.Required(),
			mcp.Description("JSON array of choices or comma-separated string (e.g. '[\"Pizza\", \"Tacos\"]'). Min 2 options."),
		),
		mcp.WithNumber("expiration",
			mcp.Description("Duration in seconds before poll closes. Default 86400 (24h)."),
		),
	)
	s.AddTool(createPollTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		subject, ok := getArgs(request)["subject"].(string)
		if !ok || subject == "" {
			return mcp.NewToolResultError("subject is required"), nil
		}

		optionsStr, ok := getArgs(request)["options"].(string)
		if !ok || optionsStr == "" {
			return mcp.NewToolResultError("options are required as a JSON array string"),
				nil
		}

		var options []string
		if err := json.Unmarshal([]byte(optionsStr), &options); err != nil {
			// Try to handle if it's passed as a simple comma-separated string as fallback
			if strings.Contains(optionsStr, ",") {
				parts := strings.Split(optionsStr, ",")
				for _, p := range parts {
					options = append(options, strings.TrimSpace(p))
				}
			} else {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to parse options JSON: %v. Please provide a valid JSON array string like '[\"Option 1\", \"Option 2\"]'", err)), nil
			}
		}

		if len(options) < 2 {
			return mcp.NewToolResultError("At least 2 options are required"), nil
		}

		expiration := 0
		if e, ok := getArgs(request)["expiration"].(float64); ok {
			expiration = int(e)
		}

		poll, err := c.CreatePoll(ctx, groupID, subject, options, expiration)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create poll: %v", err)), nil
		}

		result, err := json.MarshalIndent(poll, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Get poll tool
	getPollTool := mcp.NewTool("groupme_get_poll",
		mcp.WithDescription("Get detailed poll results including vote counts."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group. Get this from groupme_list_groups or groupme_search_group_by_name."),
		),
		mcp.WithString("poll_id",
			mcp.Required(),
			mcp.Description("The ID of the poll. Get this from the output of groupme_list_polls."),
		),
	)
	s.AddTool(getPollTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		pollID, ok := getArgs(request)["poll_id"].(string)
		if !ok || pollID == "" {
			return mcp.NewToolResultError("poll_id is required"), nil
		}

		poll, err := c.GetPoll(ctx, groupID, pollID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get poll: %v", err)), nil
		}

		result, err := json.MarshalIndent(poll, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Vote in poll tool
	votePollTool := mcp.NewTool("groupme_vote_poll",
		mcp.WithDescription("Vote in a poll. Provide option_id(s) as string or JSON array."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group containing the poll."),
		),
		mcp.WithString("poll_id",
			mcp.Required(),
			mcp.Description("The ID of the poll to vote in. Get this from groupme_list_polls."),
		),
		mcp.WithString("option_ids",
			mcp.Required(),
			mcp.Description("The option ID(s) to vote for. For single-choice: just the ID (e.g. '1'). For multi-choice: JSON array (e.g. '[\"1\", \"2\"]')."),
		),
	)
	s.AddTool(votePollTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		pollID, ok := getArgs(request)["poll_id"].(string)
		if !ok || pollID == "" {
			return mcp.NewToolResultError("poll_id is required"), nil
		}

		optionIDsStr, ok := getArgs(request)["option_ids"].(string)
		if !ok || optionIDsStr == "" {
			return mcp.NewToolResultError("option_ids is required"), nil
		}

		// Parse option IDs - could be a single ID or JSON array
		var optionIDs []string
		if err := json.Unmarshal([]byte(optionIDsStr), &optionIDs); err != nil {
			// Not valid JSON array, treat as single ID
			optionIDs = []string{strings.TrimSpace(optionIDsStr)}
		}

		poll, err := c.VotePoll(ctx, groupID, pollID, optionIDs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to vote in poll: %v", err)), nil
		}

		result, err := json.MarshalIndent(poll, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Vote recorded successfully!\n%s", string(result))), nil
	})

	// End poll tool
	endPollTool := mcp.NewTool("groupme_end_poll",
		mcp.WithDescription("End a poll immediately. Only the creator can do this."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group containing the poll."),
		),
		mcp.WithString("poll_id",
			mcp.Required(),
			mcp.Description("The ID of the poll to end. Get this from groupme_list_polls."),
		),
	)
	s.AddTool(endPollTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		pollID, ok := getArgs(request)["poll_id"].(string)
		if !ok || pollID == "" {
			return mcp.NewToolResultError("poll_id is required"), nil
		}

		poll, err := c.EndPoll(ctx, groupID, pollID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to end poll: %v", err)), nil
		}

		result, err := json.MarshalIndent(poll, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Poll ended successfully!\n%s", string(result))), nil
	})
}
