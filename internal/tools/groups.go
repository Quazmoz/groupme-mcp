// Package tools contains MCP tool implementations for GroupMe.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterGroupTools registers group-related MCP tools.
func RegisterGroupTools(s *server.MCPServer, c *client.Client) {
	// List groups tool - PRIMARY TOOL for seeing all groups
	listGroupsTool := mcp.NewTool("groupme_list_groups",
		mcp.WithDescription("List all GroupMe groups. Returns group id and name."),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	s.AddTool(listGroupsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groups, err := listGroupSummaries(ctx, c)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list groups: %v", err)), nil
		}

		result, err := json.MarshalIndent(groups, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Search group by name tool
	searchGroupTool := mcp.NewTool("groupme_search_group_by_name",
		mcp.WithDescription("Search for a group by name. Returns matching groups with id. For subtopics, use groupme_get_group_subtopics on the parent group, then read subtopic messages via /groups/{subgroup_id}/messages."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group to search for. Case-insensitive."),
		),
	)
	s.AddTool(searchGroupTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		searchName, ok := getArgs(request)["name"].(string)
		if !ok || searchName == "" {
			return mcp.NewToolResultError("name is required"), nil
		}

		responseObj, err := searchGroupSummaries(ctx, c, searchName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list groups: %v", err)), nil
		}

		if responseObj.MatchCount == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No groups found matching '%s'", searchName)), nil
		}

		result, err := json.MarshalIndent(responseObj, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Get group tool
	getGroupTool := mcp.NewTool("groupme_get_group",
		mcp.WithDescription("Get group details: members, message count, share URL."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group to retrieve. Get this from groupme_list_groups or groupme_search_group_by_name."),
		),
	)
	s.AddTool(getGroupTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		group, err := c.GetGroup(ctx, groupID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get group: %v", err)), nil
		}

		result, err := json.MarshalIndent(group, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Get group tool RAW
	getGroupRawTool := mcp.NewTool("groupme_get_group_raw",
		mcp.WithDescription("Get raw JSON data for a group as returned by GroupMe API."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group to retrieve."),
		),
	)
	s.AddTool(getGroupRawTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		rawData, err := c.GetGroupRaw(ctx, groupID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get group raw: %v", err)), nil
		}

		return mcp.NewToolResultText(string(rawData)), nil
	})

	// Get group subtopics tool
	getSubtopicsTool := mcp.NewTool("groupme_get_group_subtopics",
		mcp.WithDescription("Get subtopics (channels) of a parent group via /groups/{group_id}/subgroups. The returned subgroup IDs are then used with /groups/{subgroup_id}/messages to read subtopic messages."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the main group to find topics in."),
		),
		mcp.WithString("subtopic_name",
			mcp.Description("Optional exact or partial subtopic name to highlight in the result, such as 'Online Mission'."),
		),
	)
	s.AddTool(getSubtopicsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		subtopicName, _ := getArgs(request)["subtopic_name"].(string)
		lookup, err := buildSubtopicLookup(ctx, c, groupID, subtopicName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get group subtopics: %v", err)), nil
		}

		result, err := json.MarshalIndent(lookup, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Create group tool
	createGroupTool := mcp.NewTool("groupme_create_group",
		mcp.WithDescription("Create a new group. You become the admin."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the group (max 140 characters)"),
		),
		mcp.WithString("description",
			mcp.Description("Description of the group (optional)"),
		),
	)
	s.AddTool(createGroupTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		name, ok := getArgs(request)["name"].(string)
		if !ok || name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}

		description := ""
		if desc, ok := getArgs(request)["description"].(string); ok {
			description = desc
		}

		group, err := c.CreateGroup(ctx, name, description)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create group: %v", err)), nil
		}

		result, err := json.MarshalIndent(group, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Update group tool
	updateGroupTool := mcp.NewTool("groupme_update_group",
		mcp.WithDescription("Update a group's name, description, or image."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group to update"),
		),
		mcp.WithString("name",
			mcp.Description("New name for the group (max 140 characters)"),
		),
		mcp.WithString("description",
			mcp.Description("New description for the group"),
		),
		mcp.WithString("image_url",
			mcp.Description("New image URL (must be from GroupMe image service)"),
		),
	)
	s.AddTool(updateGroupTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		name := ""
		if n, ok := getArgs(request)["name"].(string); ok {
			name = n
		}
		description := ""
		if d, ok := getArgs(request)["description"].(string); ok {
			description = d
		}
		imageURL := ""
		if img, ok := getArgs(request)["image_url"].(string); ok {
			imageURL = img
		}

		group, err := c.UpdateGroup(ctx, groupID, name, description, imageURL)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update group: %v", err)), nil
		}

		result, err := json.MarshalIndent(group, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Destroy group tool
	destroyGroupTool := mcp.NewTool("groupme_destroy_group",
		mcp.WithDescription("Permanently delete a group. Only the creator can do this."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group to delete"),
		),
	)
	s.AddTool(destroyGroupTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		if err := c.DestroyGroup(ctx, groupID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to destroy group: %v", err)), nil
		}

		return mcp.NewToolResultText("Group deleted successfully"), nil
	})

	// List former groups tool
	listFormerGroupsTool := mcp.NewTool("groupme_list_former_groups",
		mcp.WithDescription("List groups you left but can rejoin."),
		mcp.WithReadOnlyHintAnnotation(true),
	)
	s.AddTool(listFormerGroupsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groups, err := c.ListFormerGroups(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list former groups: %v", err)), nil
		}

		result, err := json.MarshalIndent(groups, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Remove member tool
	removeMemberTool := mcp.NewTool("groupme_remove_member",
		mcp.WithDescription("Remove a member from a group by membership_id."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group"),
		),
		mcp.WithString("membership_id",
			mcp.Required(),
			mcp.Description("The membership ID of the member to remove (different from user_id)"),
		),
	)
	s.AddTool(removeMemberTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		membershipID, ok := getArgs(request)["membership_id"].(string)
		if !ok || membershipID == "" {
			return mcp.NewToolResultError("membership_id is required"), nil
		}

		if err := c.RemoveMember(ctx, groupID, membershipID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to remove member: %v", err)), nil
		}

		return mcp.NewToolResultText("Member removed successfully"), nil
	})

	// Update nickname tool
	updateNicknameTool := mcp.NewTool("groupme_update_nickname",
		mcp.WithDescription("Change your nickname in a group."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group"),
		),
		mcp.WithString("nickname",
			mcp.Required(),
			mcp.Description("Your new nickname (1-50 characters)"),
		),
	)
	s.AddTool(updateNicknameTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		nickname, ok := getArgs(request)["nickname"].(string)
		if !ok || nickname == "" {
			return mcp.NewToolResultError("nickname is required"), nil
		}

		member, err := c.UpdateNickname(ctx, groupID, nickname)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update nickname: %v", err)), nil
		}

		result, err := json.MarshalIndent(member, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// COMBO TOOL: Search group by name AND get messages in one call
	getGroupMessagesTool := mcp.NewTool("groupme_get_group_messages",
		mcp.WithDescription("Get messages from a group by name. Resolves name to ID automatically."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group to search for. Alias: name."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of messages to retrieve (default 20, max 100)"),
		),
	)
	s.AddTool(getGroupMessagesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		searchName := getFirstStringArg(args, "group_name", "name")
		if searchName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
		}

		limit := getIntArg(args, "limit", 20, 1, 100)

		// First, find the group
		groups, err := c.ListAllGroups(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list groups: %v", err)), nil
		}

		var matchedGroup *client.Group
		searchLower := strings.ToLower(searchName)
		for i, g := range groups {
			if strings.Contains(strings.ToLower(g.Name), searchLower) {
				matchedGroup = &groups[i]
				break
			}
		}

		if matchedGroup == nil {
			return mcp.NewToolResultText(fmt.Sprintf("No group found matching '%s'", searchName)), nil
		}

		// Now get messages from that group
		messages, err := c.ListMessages(ctx, matchedGroup.ID, limit, "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Found group '%s' (ID: %s) but failed to get messages: %v", matchedGroup.Name, matchedGroup.ID, err)), nil
		}

		response := struct {
			Group    client.Group     `json:"group"`
			Messages []client.Message `json:"messages"`
		}{
			Group:    *matchedGroup,
			Messages: messages,
		}

		result, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Leave group tool
	leaveGroupTool := mcp.NewTool("groupme_leave_group",
		mcp.WithDescription("Leave a group. Requires your membership_id from groupme_get_group."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group to leave"),
		),
		mcp.WithString("membership_id",
			mcp.Required(),
			mcp.Description("Your membership ID in that group (different from your user_id)"),
		),
	)
	s.AddTool(leaveGroupTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		membershipID, ok := getArgs(request)["membership_id"].(string)
		if !ok || membershipID == "" {
			return mcp.NewToolResultError("membership_id is required"), nil
		}

		if err := c.LeaveGroup(ctx, groupID, membershipID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to leave group: %v", err)), nil
		}

		return mcp.NewToolResultText("Successfully left the group"), nil
	})

	// Rejoin group tool
	rejoinGroupTool := mcp.NewTool("groupme_rejoin_group",
		mcp.WithDescription("Rejoin a group you previously left."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the former group to rejoin"),
		),
	)
	s.AddTool(rejoinGroupTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		group, err := c.RejoinGroup(ctx, groupID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to rejoin group: %v", err)), nil
		}

		result, err := json.MarshalIndent(group, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Add members tool
	addMembersTool := mcp.NewTool("groupme_add_members",
		mcp.WithDescription("Add members to a group by user_id, phone, or email."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group to add members to"),
		),
		mcp.WithString("user_id",
			mcp.Description("The user_id of the member to add"),
		),
		mcp.WithString("phone",
			mcp.Description("Phone number of the member to add (format: +1 555 555 5555)"),
		),
		mcp.WithString("email",
			mcp.Description("Email of the member to add"),
		),
		mcp.WithString("nickname",
			mcp.Required(),
			mcp.Description("Nickname for the new member in this group"),
		),
	)
	s.AddTool(addMembersTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		nickname, ok := getArgs(request)["nickname"].(string)
		if !ok || nickname == "" {
			return mcp.NewToolResultError("nickname is required"), nil
		}

		member := client.MemberToAdd{Nickname: nickname}
		if uid, ok := getArgs(request)["user_id"].(string); ok && uid != "" {
			member.UserID = uid
		}
		if phone, ok := getArgs(request)["phone"].(string); ok && phone != "" {
			member.Phone = phone
		}
		if email, ok := getArgs(request)["email"].(string); ok && email != "" {
			member.Email = email
		}

		if member.UserID == "" && member.Phone == "" && member.Email == "" {
			return mcp.NewToolResultError("At least one of user_id, phone, or email is required"), nil
		}

		resultsID, err := c.AddMembers(ctx, groupID, []client.MemberToAdd{member})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to add member: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Member add request submitted. Results ID: %s", resultsID)), nil
	})

	// Get add members results tool
	getAddResultsTool := mcp.NewTool("groupme_get_add_results",
		mcp.WithDescription("Check status of an add-members request."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The group ID"),
		),
		mcp.WithString("results_id",
			mcp.Required(),
			mcp.Description("The results_id from the add members request"),
		),
	)
	s.AddTool(getAddResultsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		resultsID, ok := getArgs(request)["results_id"].(string)
		if !ok || resultsID == "" {
			return mcp.NewToolResultError("results_id is required"), nil
		}

		members, err := c.GetAddMembersResults(ctx, groupID, resultsID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get results: %v", err)), nil
		}

		result, err := json.MarshalIndent(members, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// COMBO TOOL: Send message to group by name
	sendToGroupByNameTool := mcp.NewTool("groupme_send_to_group_by_name",
		mcp.WithDescription("Send a message to a group by name. Resolves name to ID automatically."),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group"),
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The message text to send"),
		),
	)
	s.AddTool(sendToGroupByNameTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupName, ok := getArgs(request)["group_name"].(string)
		if !ok || groupName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
		}

		text, ok := getArgs(request)["text"].(string)
		if !ok || text == "" {
			return mcp.NewToolResultError("text is required"), nil
		}

		// Find the group by name
		group, err := c.SearchGroupByName(ctx, groupName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Could not find group '%s': %v", groupName, err)), nil
		}

		// Send message
		sourceGUID := fmt.Sprintf("%d", time.Now().UnixNano())
		message, err := c.SendMessage(ctx, group.ID, text, sourceGUID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Found group '%s' but failed to send message: %v", group.Name, err)), nil
		}

		result, err := json.MarshalIndent(message, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Message sent to group '%s'!\n%s", group.Name, string(result))), nil
	})

	// List group members tool - helps AI see who's in a group before mentioning
	listMembersTool := mcp.NewTool("groupme_list_group_members",
		mcp.WithDescription("List all members of a group by name."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the group"),
		),
	)
	s.AddTool(listMembersTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupName, ok := getArgs(request)["group_name"].(string)
		if !ok || groupName == "" {
			return mcp.NewToolResultError("group_name is required"), nil
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

		// Format member list
		type MemberInfo struct {
			Nickname string `json:"nickname"`
			UserID   string `json:"user_id"`
		}
		var members []MemberInfo
		for _, m := range fullGroup.Members {
			members = append(members, MemberInfo{Nickname: m.Nickname, UserID: m.UserID})
		}

		result, err := json.MarshalIndent(struct {
			GroupName   string       `json:"group_name"`
			GroupID     string       `json:"group_id"`
			MemberCount int          `json:"member_count"`
			Members     []MemberInfo `json:"members"`
		}{
			GroupName:   fullGroup.Name,
			GroupID:     fullGroup.ID,
			MemberCount: len(members),
			Members:     members,
		}, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Who is tool - find a person across all groups and DMs
	whoIsTool := mcp.NewTool("groupme_who_is",
		mcp.WithDescription("Find a person by name across all groups and DMs."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The name (or partial name) of the person to find"),
		),
	)
	s.AddTool(whoIsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		name, ok := getArgs(request)["name"].(string)
		if !ok || name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}

		nameLower := strings.ToLower(name)

		type FoundIn struct {
			Type     string `json:"type"` // "group" or "dm"
			Name     string `json:"name"`
			ID       string `json:"id"`
			Nickname string `json:"nickname,omitempty"`
			UserID   string `json:"user_id"`
		}
		var results []FoundIn

		// Search in groups
		groups, err := c.ListAllGroups(ctx)
		if err == nil {
			for _, group := range groups {
				fullGroup, err := c.GetGroup(ctx, group.ID)
				if err != nil {
					continue
				}
				for _, member := range fullGroup.Members {
					if strings.Contains(strings.ToLower(member.Nickname), nameLower) {
						results = append(results, FoundIn{
							Type:     "group",
							Name:     group.Name,
							ID:       group.ID,
							Nickname: member.Nickname,
							UserID:   member.UserID,
						})
					}
				}
			}
		}

		// Search in DM chats
		chats, err := c.ListChats(ctx, 1, 100)
		if err == nil {
			for _, chat := range chats {
				if strings.Contains(strings.ToLower(chat.OtherUser.Name), nameLower) {
					results = append(results, FoundIn{
						Type:   "dm",
						Name:   chat.OtherUser.Name,
						ID:     chat.OtherUser.ID,
						UserID: chat.OtherUser.ID,
					})
				}
			}
		}

		if len(results) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No person named '%s' found in your groups or DM chats.", name)), nil
		}

		result, err := json.MarshalIndent(struct {
			SearchName string    `json:"searched_for"`
			Found      int       `json:"found_count"`
			Results    []FoundIn `json:"results"`
		}{
			SearchName: name,
			Found:      len(results),
			Results:    results,
		}, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// List matching groups - ONLY for searching when user specifies a name
	listMatchingGroupsTool := mcp.NewTool("groupme_list_matching_groups",
		mcp.WithDescription("Find groups matching a name. For subtopics, list them from the parent group's /subgroups route, then read messages via /groups/{subgroup_id}/messages."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("The specific group name to search for (required - do not pass empty string)"),
		),
	)
	s.AddTool(listMatchingGroupsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		name, _ := getArgs(request)["name"].(string)

		groups, err := listGroupSummaries(ctx, c)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list groups: %v", err)), nil
		}

		// If no name provided, return ALL groups (fallback for AI mistakes)
		if name == "" {
			result, err := json.MarshalIndent(struct {
				Note   string         `json:"note"`
				Count  int            `json:"count"`
				Groups []groupSummary `json:"groups"`
				Hint   string         `json:"ai_hint"`
			}{
				Note:   "Showing all groups (no search filter provided)",
				Count:  len(groups),
				Groups: groups,
				Hint:   "If you are looking for a sub-topic or channel within one of these groups, use 'groupme_get_group_subtopics' with the parent group ID. Then use the returned subgroup_id with 'groupme_list_subtopic_messages', which reads via /groups/{subgroup_id}/messages.",
			}, "", "  ")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
			}
			return mcp.NewToolResultText(string(result)), nil
		}

		searchResult, err := searchGroupSummaries(ctx, c, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to search groups: %v", err)), nil
		}
		matches := searchResult.Matches

		if len(matches) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No groups found matching '%s'", name)), nil
		}

		if len(matches) == 1 {
			return mcp.NewToolResultText(fmt.Sprintf("Found 1 group matching '%s': %s (ID: %s)", name, matches[0].Name, matches[0].ID)), nil
		}

		result, err := json.MarshalIndent(struct {
			SearchName  string         `json:"searched_for"`
			Found       int            `json:"found_count"`
			Warning     string         `json:"warning,omitempty"`
			Matches     []groupSummary `json:"matches"`
			AI_Guidance string         `json:"ai_guidance,omitempty"`
		}{
			SearchName:  name,
			Found:       len(matches),
			Warning:     "Multiple groups match this name. Please specify which one you mean.",
			Matches:     matches,
			AI_Guidance: "If you are looking for a sub-topic or channel within one of these groups, use 'groupme_get_group_subtopics' with the parent group ID. Then use the returned subgroup_id with 'groupme_list_subtopic_messages', which reads via /groups/{subgroup_id}/messages.",
		}, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Create subtopic tool
	createSubtopicTool := mcp.NewTool("groupme_create_subtopic",
		mcp.WithDescription("Create a subtopic (channel) within a group."),
		mcp.WithString("group_id", mcp.Required(), mcp.Description("The ID of the parent group.")),
		mcp.WithString("topic", mcp.Required(), mcp.Description("The name of the new subtopic.")),
		mcp.WithString("description", mcp.Description("An optional description for the subtopic.")),
		mcp.WithString("group_type", mcp.Description("Can be 'private' (anyone can post) or 'announcement' (only admins can post). Defaults to private.")),
	)
	s.AddTool(createSubtopicTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, _ := getArgs(request)["group_id"].(string)
		topic, _ := getArgs(request)["topic"].(string)
		if groupID == "" || topic == "" {
			return mcp.NewToolResultError("group_id and topic are required"), nil
		}
		description, _ := getArgs(request)["description"].(string)
		groupType, _ := getArgs(request)["group_type"].(string)

		sg, err := c.CreateSubgroup(ctx, groupID, topic, description, groupType)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create subtopic: %v", err)), nil
		}
		res, _ := json.MarshalIndent(sg, "", "  ")
		return mcp.NewToolResultText(string(res)), nil
	})

	// Update subtopic tool
	updateSubtopicTool := mcp.NewTool("groupme_update_subtopic",
		mcp.WithDescription("Update a subtopic's name, description, or type."),
		mcp.WithString("group_id", mcp.Required(), mcp.Description("The ID of the parent group.")),
		mcp.WithString("subgroup_id", mcp.Required(), mcp.Description("The ID of the subtopic.")),
		mcp.WithString("topic", mcp.Description("The new name of the subtopic.")),
		mcp.WithString("description", mcp.Description("The new description for the subtopic.")),
		mcp.WithString("group_type", mcp.Description("Can be 'private' or 'announcement'.")),
	)
	s.AddTool(updateSubtopicTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, _ := getArgs(request)["group_id"].(string)
		subgroupID, _ := getArgs(request)["subgroup_id"].(string)
		if groupID == "" || subgroupID == "" {
			return mcp.NewToolResultError("group_id and subgroup_id are required"), nil
		}
		topic, _ := getArgs(request)["topic"].(string)
		description, _ := getArgs(request)["description"].(string)
		groupType, _ := getArgs(request)["group_type"].(string)

		sg, err := c.UpdateSubgroup(ctx, groupID, subgroupID, topic, description, groupType)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update subtopic: %v", err)), nil
		}
		res, _ := json.MarshalIndent(sg, "", "  ")
		return mcp.NewToolResultText(string(res)), nil
	})

	// Delete subtopic tool
	deleteSubtopicTool := mcp.NewTool("groupme_delete_subtopic",
		mcp.WithDescription("Delete a subtopic."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("group_id", mcp.Required(), mcp.Description("The ID of the parent group.")),
		mcp.WithString("subgroup_id", mcp.Required(), mcp.Description("The ID of the subtopic to delete.")),
	)
	s.AddTool(deleteSubtopicTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, _ := getArgs(request)["group_id"].(string)
		subgroupID, _ := getArgs(request)["subgroup_id"].(string)
		if groupID == "" || subgroupID == "" {
			return mcp.NewToolResultError("group_id and subgroup_id are required"), nil
		}
		if err := c.DeleteSubgroup(ctx, groupID, subgroupID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete subtopic: %v", err)), nil
		}
		return mcp.NewToolResultText("Subtopic successfully deleted"), nil
	})

	// Mute subtopic tool
	muteSubtopicTool := mcp.NewTool("groupme_mute_subtopic",
		mcp.WithDescription("Mute a subtopic's notifications for yourself."),
		mcp.WithString("group_id", mcp.Required(), mcp.Description("The ID of the parent group.")),
		mcp.WithString("subgroup_id", mcp.Required(), mcp.Description("The ID of the subtopic to mute.")),
		mcp.WithNumber("duration", mcp.Description("The length of time in minutes to mute notifications. Send -1 to mute forever.")),
	)
	s.AddTool(muteSubtopicTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, _ := getArgs(request)["group_id"].(string)
		subgroupID, _ := getArgs(request)["subgroup_id"].(string)
		if groupID == "" || subgroupID == "" {
			return mcp.NewToolResultError("group_id and subgroup_id are required"), nil
		}

		durationMinutes := -1
		if d, ok := getArgs(request)["duration"].(float64); ok {
			durationMinutes = int(d)
		}

		if err := c.MuteSubgroup(ctx, groupID, subgroupID, durationMinutes); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to mute subtopic: %v", err)), nil
		}
		return mcp.NewToolResultText("Subtopic successfully muted"), nil
	})

	// Unmute subtopic tool
	unmuteSubtopicTool := mcp.NewTool("groupme_unmute_subtopic",
		mcp.WithDescription("Unmute a previously muted subtopic."),
		mcp.WithString("group_id", mcp.Required(), mcp.Description("The ID of the parent group.")),
		mcp.WithString("subgroup_id", mcp.Required(), mcp.Description("The ID of the subtopic to unmute.")),
	)
	s.AddTool(unmuteSubtopicTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, _ := getArgs(request)["group_id"].(string)
		subgroupID, _ := getArgs(request)["subgroup_id"].(string)
		if groupID == "" || subgroupID == "" {
			return mcp.NewToolResultError("group_id and subgroup_id are required"), nil
		}

		if err := c.UnmuteSubgroup(ctx, groupID, subgroupID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to unmute subtopic: %v", err)), nil
		}
		return mcp.NewToolResultText("Subtopic successfully unmuted"), nil
	})

	// Change nickname tool
	changeNicknameTool := mcp.NewTool("groupme_change_nickname",
		mcp.WithDescription("Change a member's nickname. Requires group owner privileges."),
		mcp.WithString("group_id", mcp.Required(), mcp.Description("The ID of the group.")),
		mcp.WithString("membership_id", mcp.Required(), mcp.Description("The membership ID of the user to rename (find this in the group members list, it is 'id' inside the member array, NOT user_id).")),
		mcp.WithString("nickname", mcp.Required(), mcp.Description("The new nickname.")),
	)
	s.AddTool(changeNicknameTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, _ := getArgs(request)["group_id"].(string)
		membershipID, _ := getArgs(request)["membership_id"].(string)
		nickname, _ := getArgs(request)["nickname"].(string)
		if groupID == "" || membershipID == "" || nickname == "" {
			return mcp.NewToolResultError("group_id, membership_id, and nickname are required"), nil
		}

		member, err := c.UpdateMemberNickname(ctx, groupID, membershipID, nickname)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to change nickname: %v", err)), nil
		}

		res, _ := json.MarshalIndent(member, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Nickname updated:\n%s", string(res))), nil
	})
}
