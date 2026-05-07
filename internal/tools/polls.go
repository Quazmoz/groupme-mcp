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
		mcp.WithDescription("Create a poll with a question and options (min 2).\npoll_type=\"single\" means one answer only. poll_type=\"multi\" means multiple answers allowed.\nvisibility=\"public\" means voters are visible. visibility=\"anonymous\" means voters are hidden.\nFor scheduled automations, prefer expiration_unix to avoid timezone/duration mistakes."),
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
			mcp.Description("Relative duration in seconds before poll closes."),
		),
		mcp.WithNumber("expiration_unix",
			mcp.Description("Absolute Unix timestamp in seconds for poll expiration."),
		),
		mcp.WithString("poll_type",
			mcp.Description("\"single\" or \"multi\" (default \"multi\")."),
		),
		mcp.WithString("visibility",
			mcp.Description("\"public\" or \"anonymous\" (default \"public\")."),
		),
	)
	s.AddTool(createPollTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		poll, err := createPollWithArgs(ctx, c, getArgs(request))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create poll: %v", err)), nil
		}

		result, err := json.MarshalIndent(poll, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Create poll by group name tool
	createPollByNameTool := mcp.NewTool("groupme_create_poll_by_group_name",
		mcp.WithDescription("Create a poll by resolving the group name first."),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name of the group."),
		),
		mcp.WithString("subject",
			mcp.Required(),
			mcp.Description("The question to ask."),
		),
		mcp.WithString("options",
			mcp.Required(),
			mcp.Description("JSON array of choices or comma-separated string. Min 2 options."),
		),
		mcp.WithNumber("expiration",
			mcp.Description("Relative duration in seconds before poll closes."),
		),
		mcp.WithNumber("expiration_unix",
			mcp.Description("Absolute Unix timestamp in seconds for poll expiration."),
		),
		mcp.WithString("poll_type",
			mcp.Description("\"single\" or \"multi\" (default \"multi\")."),
		),
		mcp.WithString("visibility",
			mcp.Description("\"public\" or \"anonymous\" (default \"public\")."),
		),
	)
	s.AddTool(createPollByNameTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		poll, group, err := createPollByGroupNameWithArgs(ctx, c, getArgs(request))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create poll: %v", err)), nil
		}

		result, err := json.MarshalIndent(poll, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Created in group %s (ID: %s)\n%s", group.Name, group.ID, string(result))), nil
	})

	// Create poll in subgroup by name tool
	createPollInSubgroupByNameTool := mcp.NewTool("groupme_create_poll_in_subgroup_by_name",
		mcp.WithDescription("Create a poll in a subgroup/topic by resolving the parent group name and subgroup topic."),
		mcp.WithString("parent_group_name",
			mcp.Required(),
			mcp.Description("The name of the parent group."),
		),
		mcp.WithString("subgroup_topic",
			mcp.Required(),
			mcp.Description("The name of the subgroup/topic. Emoji and whitespace are normalized for matching."),
		),
		mcp.WithString("subject",
			mcp.Required(),
			mcp.Description("The question to ask."),
		),
		mcp.WithString("options",
			mcp.Required(),
			mcp.Description("JSON array of choices or comma-separated string. Min 2 options."),
		),
		mcp.WithNumber("expiration",
			mcp.Description("Relative duration in seconds before poll closes."),
		),
		mcp.WithNumber("expiration_unix",
			mcp.Description("Absolute Unix timestamp in seconds for poll expiration."),
		),
		mcp.WithString("poll_type",
			mcp.Description("\"single\" or \"multi\" (default \"multi\")."),
		),
		mcp.WithString("visibility",
			mcp.Description("\"public\" or \"anonymous\" (default \"public\")."),
		),
	)
	s.AddTool(createPollInSubgroupByNameTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		poll, group, subgroup, err := createPollInSubgroupByNameWithArgs(ctx, c, getArgs(request))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create poll: %v", err)), nil
		}

		result, err := json.MarshalIndent(poll, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Created in group %s (ID: %s), subgroup %s (ID: %s)\n%s", group.Name, group.ID, subgroup.Topic, getSubgroupID(subgroup), string(result))), nil
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
		poll, err := endPollWithArgs(ctx, c, getArgs(request))
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

// parseCreatePollArgs extracts the common arguments for creating a poll
func parseCreatePollArgs(request mcp.CallToolRequest) (client.CreatePollRequest, error) {
	return parseCreatePollArgsMap(getArgs(request))
}

func parseCreatePollArgsMap(args map[string]interface{}) (client.CreatePollRequest, error) {
	var req client.CreatePollRequest

	subject, ok := args["subject"].(string)
	if !ok || subject == "" {
		return req, fmt.Errorf("subject is required")
	}
	req.Subject = subject

	optionsStr, ok := args["options"].(string)
	if !ok || optionsStr == "" {
		return req, fmt.Errorf("options are required")
	}

	var options []string
	if err := json.Unmarshal([]byte(optionsStr), &options); err != nil {
		if strings.Contains(optionsStr, ",") {
			parts := strings.Split(optionsStr, ",")
			for _, p := range parts {
				options = append(options, strings.TrimSpace(p))
			}
		} else {
			return req, fmt.Errorf("failed to parse options JSON: %v", err)
		}
	}
	req.Options = options

	if e, ok := args["expiration"].(float64); ok {
		req.ExpirationSecs = int(e)
	}

	if e, ok := args["expiration_unix"].(float64); ok {
		req.ExpirationUnix = int64(e)
	}

	if pt, ok := args["poll_type"].(string); ok && pt != "" {
		req.PollType = pt
	}

	if vis, ok := args["visibility"].(string); ok && vis != "" {
		req.Visibility = vis
	}

	return req, nil
}

func createPollWithArgs(ctx context.Context, c *client.Client, args map[string]interface{}) (*client.Poll, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}

	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}

	reqObj, err := parseCreatePollArgsMap(args)
	if err != nil {
		return nil, err
	}
	reqObj.GroupID = groupID

	return c.CreatePollWithRequest(ctx, reqObj)
}

func createPollByGroupNameWithArgs(ctx context.Context, c *client.Client, args map[string]interface{}) (*client.Poll, *client.Group, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, nil, err
	}

	groupName := getString(args, "group_name")
	if groupName == "" {
		return nil, nil, fmt.Errorf("group_name is required")
	}

	reqObj, err := parseCreatePollArgsMap(args)
	if err != nil {
		return nil, nil, err
	}

	group, err := resolveGroupForWrite(ctx, c, groupName)
	if err != nil {
		return nil, nil, err
	}
	reqObj.GroupID = group.ID

	poll, err := c.CreatePollWithRequest(ctx, reqObj)
	if err != nil {
		return nil, nil, err
	}

	return poll, group, nil
}

func createPollInSubgroupByNameWithArgs(ctx context.Context, c *client.Client, args map[string]interface{}) (*client.Poll, *client.Group, *client.Subgroup, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, nil, nil, err
	}

	parentGroupName := getString(args, "parent_group_name")
	if parentGroupName == "" {
		return nil, nil, nil, fmt.Errorf("parent_group_name is required")
	}

	subgroupTopic := getString(args, "subgroup_topic")
	if subgroupTopic == "" {
		return nil, nil, nil, fmt.Errorf("subgroup_topic is required")
	}

	reqObj, err := parseCreatePollArgsMap(args)
	if err != nil {
		return nil, nil, nil, err
	}

	group, err := resolveGroupForWrite(ctx, c, parentGroupName)
	if err != nil {
		return nil, nil, nil, err
	}

	subgroups, err := c.ListAllSubgroups(ctx, group.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list subgroups: %w", err)
	}

	subgroup, err := findSubgroup(subgroups, subgroupTopic)
	if err != nil {
		return nil, nil, nil, err
	}

	reqObj.GroupID = getSubgroupID(subgroup)

	poll, err := c.CreatePollWithRequest(ctx, reqObj)
	if err != nil {
		return nil, nil, nil, err
	}

	return poll, group, subgroup, nil
}

func endPollWithArgs(ctx context.Context, c *client.Client, args map[string]interface{}) (*client.Poll, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}

	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}

	pollID := getString(args, "poll_id")
	if pollID == "" {
		return nil, fmt.Errorf("poll_id is required")
	}

	return c.EndPoll(ctx, groupID, pollID)
}

func resolveGroupForWrite(ctx context.Context, c *client.Client, groupName string) (*client.Group, error) {
	groups, err := c.ListAllGroupsWithOptions(ctx, true)
	if err != nil {
		return nil, err
	}

	search := strings.ToLower(strings.TrimSpace(groupName))
	if search == "" {
		return nil, fmt.Errorf("group name is required")
	}

	var exactMatches []client.Group
	var partialMatches []client.Group
	for _, g := range groups {
		name := strings.ToLower(strings.TrimSpace(g.Name))
		switch {
		case name == search:
			exactMatches = append(exactMatches, g)
		case strings.Contains(name, search):
			partialMatches = append(partialMatches, g)
		}
	}

	if len(exactMatches) == 1 {
		return &exactMatches[0], nil
	}
	if len(exactMatches) > 1 {
		return nil, fmt.Errorf("multiple groups exactly matched %q: %s", groupName, formatGroupCandidates(exactMatches))
	}
	if len(partialMatches) == 1 {
		return &partialMatches[0], nil
	}
	if len(partialMatches) > 1 {
		return nil, fmt.Errorf("multiple groups matched %q: %s", groupName, formatGroupCandidates(partialMatches))
	}

	return nil, fmt.Errorf("no group found matching %q", groupName)
}

func formatGroupCandidates(groups []client.Group) string {
	candidates := make([]string, 0, len(groups))
	for _, g := range groups {
		candidates = append(candidates, fmt.Sprintf("%s (ID: %s)", g.Name, g.ID))
	}
	return strings.Join(candidates, ", ")
}

func findSubgroup(subgroups []client.Subgroup, targetTopic string) (*client.Subgroup, error) {
	// 1. Exact match
	for i, sg := range subgroups {
		if sg.Topic == targetTopic {
			return &subgroups[i], nil
		}
	}

	// 2. Case-insensitive trimmed match
	targetLower := strings.ToLower(strings.TrimSpace(targetTopic))
	for i, sg := range subgroups {
		if strings.ToLower(strings.TrimSpace(sg.Topic)) == targetLower {
			return &subgroups[i], nil
		}
	}

	// 3. Alphanumeric match
	targetAlnum := extractAlnum(targetTopic)
	if targetAlnum != "" {
		var matches []*client.Subgroup
		for i, sg := range subgroups {
			if extractAlnum(sg.Topic) == targetAlnum {
				matches = append(matches, &subgroups[i])
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		} else if len(matches) > 1 {
			var names []string
			for _, m := range matches {
				names = append(names, m.Topic)
			}
			return nil, fmt.Errorf("multiple subgroups matched '%s': %s", targetTopic, strings.Join(names, ", "))
		}
	}

	return nil, fmt.Errorf("no subgroup found matching topic '%s'", targetTopic)
}

func extractAlnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func getSubgroupID(sg *client.Subgroup) string {
	switch v := sg.ID.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
