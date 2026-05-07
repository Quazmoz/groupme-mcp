package client

import (
	"fmt"
	"net/url"
	"strings"
)

const SubgroupRoutingGuidance = "Subgroups/subtopics are accessed under the groups namespace. Use /groups/{parent_group_id}/subgroups to list them, then use /groups/{subgroup_id}/messages to read subgroup messages. Do not use /conversations/{id}/messages for subgroup message retrieval."

// EndpointKnowledge captures the canonical route shape and operator guidance
// for a known GroupMe endpoint family.
type EndpointKnowledge struct {
	Key        string `json:"key"`
	Method     string `json:"method"`
	Template   string `json:"template"`
	Summary    string `json:"summary"`
	Notes      string `json:"notes,omitempty"`
	Deprecated bool   `json:"deprecated,omitempty"`
}

var KnownEndpointFamilies = []EndpointKnowledge{
	{
		Key:      "list_groups",
		Method:   "GET",
		Template: "/groups?page={page}&per_page={per_page}",
		Summary:  "List the authenticated user's groups.",
	},
	{
		Key:      "get_current_user",
		Method:   "GET",
		Template: "/users/me",
		Summary:  "Fetch the authenticated GroupMe user profile.",
	},
	{
		Key:      "get_group",
		Method:   "GET",
		Template: "/groups/{group_id}",
		Summary:  "Fetch one group by ID.",
	},
	{
		Key:      "group_messages",
		Method:   "GET",
		Template: "/groups/{group_id}/messages",
		Summary:  "Read messages from a group-like resource.",
	},
	{
		Key:      "list_subgroups",
		Method:   "GET",
		Template: "/groups/{group_id}/subgroups",
		Summary:  "List subgroups/subtopics under a parent group.",
		Notes:    "Subgroups are discovered under the parent group's /subgroups namespace.",
	},
	{
		Key:      "get_subgroup",
		Method:   "GET",
		Template: "/groups/{group_id}/subgroups/{subgroup_id}",
		Summary:  "Fetch subgroup/subtopic metadata from its parent group namespace.",
	},
	{
		Key:      "subgroup_messages",
		Method:   "GET",
		Template: "/groups/{subgroup_id}/messages",
		Summary:  "Read subgroup/subtopic messages by treating the subgroup ID like a group ID.",
		Notes:    SubgroupRoutingGuidance,
	},
	{
		Key:        "conversation_messages",
		Method:     "GET",
		Template:   "/conversations/{id}/messages",
		Summary:    "Conversation-style message path used by some other GroupMe resource families.",
		Notes:      "Current runtime validation does not support using this path for subgroup/subtopic message reads. Prefer /groups/{subgroup_id}/messages instead.",
		Deprecated: true,
	},
}

func EndpointListGroups(page, perPage int, omitMemberships bool) string {
	values := url.Values{}
	values.Set("page", fmt.Sprintf("%d", page))
	values.Set("per_page", fmt.Sprintf("%d", perPage))
	if omitMemberships {
		values.Set("omit", "memberships")
	}
	return "/groups?" + values.Encode()
}

func EndpointGetCurrentUser() string {
	return "/users/me"
}

func EndpointGetGroup(groupID string) string {
	return fmt.Sprintf("/groups/%s", groupID)
}

func EndpointListSubgroups(groupID string, page, perPage int) string {
	values := url.Values{}
	values.Set("page", fmt.Sprintf("%d", page))
	values.Set("per_page", fmt.Sprintf("%d", perPage))
	return fmt.Sprintf("/groups/%s/subgroups?%s", groupID, values.Encode())
}

func EndpointGetSubgroup(groupID, subgroupID string) string {
	return fmt.Sprintf("/groups/%s/subgroups/%s", groupID, subgroupID)
}

func EndpointGroupMessages(groupID string, limit int, beforeID, sinceID, afterID string) string {
	values := url.Values{}
	values.Set("limit", fmt.Sprintf("%d", limit))
	if beforeID != "" {
		values.Set("before_id", beforeID)
	}
	if sinceID != "" {
		values.Set("since_id", sinceID)
	}
	if afterID != "" {
		values.Set("after_id", afterID)
	}
	return fmt.Sprintf("/groups/%s/messages?%s", groupID, values.Encode())
}

func EndpointSubgroupMessages(subgroupID string, limit int, beforeID string) string {
	return EndpointGroupMessages(subgroupID, limit, beforeID, "", "")
}

func ProbeKnowledgeForEndpoint(endpoint string) (EndpointKnowledge, []string) {
	normalized := normalizeEndpointPath(endpoint)
	notes := make([]string, 0, 2)

	switch {
	case normalized == "/users/me":
		return KnownEndpointFamilies[1], notes
	case normalized == "/groups":
		return KnownEndpointFamilies[0], notes
	case strings.HasSuffix(normalized, "/subgroups") && strings.Count(normalized, "/") == 3:
		return KnownEndpointFamilies[4], notes
	case strings.Contains(normalized, "/subgroups/") && strings.Count(normalized, "/") >= 4:
		return KnownEndpointFamilies[5], notes
	case strings.HasPrefix(normalized, "/groups/") && strings.HasSuffix(normalized, "/messages"):
		parts := strings.Split(strings.Trim(normalized, "/"), "/")
		if len(parts) == 3 {
			notes = append(notes, "Messages under /groups/{id}/messages treat the target ID as a group-like resource.")
			return KnownEndpointFamilies[3], notes
		}
	case strings.HasPrefix(normalized, "/groups/") && strings.Count(normalized, "/") == 2:
		return KnownEndpointFamilies[2], notes
	case strings.HasPrefix(normalized, "/conversations/") && strings.HasSuffix(normalized, "/messages"):
		notes = append(notes, KnownEndpointFamilies[7].Notes)
		return KnownEndpointFamilies[7], notes
	}

	return EndpointKnowledge{}, notes
}

func normalizeEndpointPath(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return ""
	}

	if parsed, err := url.Parse(trimmed); err == nil && parsed.Path != "" {
		return strings.TrimRight(parsed.Path, "/")
	}
	return strings.TrimRight(trimmed, "/")
}
