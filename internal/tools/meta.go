package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func groupToolCatalog() map[string][]string {
	return map[string][]string{
		"groups": {
			"groupme_list_groups", "groupme_search_group_by_name", "groupme_get_group", "groupme_get_group_raw",
			"groupme_get_group_subtopics", "groupme_create_group", "groupme_update_group", "groupme_destroy_group",
			"groupme_list_former_groups", "groupme_remove_member", "groupme_update_nickname", "groupme_get_group_messages",
			"groupme_leave_group", "groupme_rejoin_group", "groupme_add_members", "groupme_get_add_results",
			"groupme_send_to_group_by_name", "groupme_list_group_members", "groupme_who_is", "groupme_list_matching_groups",
			"groupme_create_subtopic", "groupme_update_subtopic", "groupme_delete_subtopic", "groupme_mute_subtopic",
			"groupme_unmute_subtopic", "groupme_change_nickname",
		},
		"messages": {
			"groupme_list_messages", "groupme_list_subtopic_messages", "groupme_get_latest_youtube_link_from_subtopic",
			"groupme_forward_latest_youtube_link_from_subtopic", "groupme_send_message", "groupme_like_message",
			"groupme_unlike_message", "groupme_search_messages", "groupme_search_in_group_by_name",
			"groupme_send_with_mention", "groupme_send_to_all", "groupme_send_image", "groupme_send_location",
			"groupme_wait_for_reply",
		},
		"dms": {
			"groupme_list_chats", "groupme_list_dm_messages", "groupme_send_dm", "groupme_get_dm_by_name", "groupme_send_dm_by_name",
		},
		"bots": {
			"groupme_list_bots", "groupme_create_bot", "groupme_post_bot_message", "groupme_destroy_bot",
		},
		"users": {
			"groupme_get_current_user", "groupme_update_user",
		},
		"blocks": {
			"groupme_list_blocks", "groupme_block_user", "groupme_unblock_user", "groupme_block_exists",
		},
		"polls": {
			"groupme_list_polls", "groupme_create_poll", "groupme_get_poll", "groupme_vote_poll", "groupme_end_poll",
		},
		"calendar": {
			"groupme_list_calendar_events", "groupme_create_event", "groupme_delete_event",
		},
		"pins": {
			"groupme_list_pinned_messages", "groupme_pin_message", "groupme_unpin_message",
		},
		"uploads": {
			"groupme_upload_image", "groupme_upload_file", "groupme_upload_video",
		},
		"reactions": {
			"groupme_react_to_message",
		},
		"gallery": {
			"groupme_list_gallery",
		},
		"meta": {
			"groupme_list_available_tools", "groupme_get_server_capabilities", "groupme_probe_api_endpoints",
		},
	}
}

func toolsForProfile(profile string) []string {
	catalog := groupToolCatalog()
	selected := []string{"meta"}

	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "core":
		selected = append(selected, "groups", "dms", "users", "bots")
	case "messaging":
		selected = append(selected, "groups", "messages", "dms", "users", "reactions")
	default:
		selected = append(selected,
			"groups", "messages", "dms", "bots", "users", "blocks", "polls", "calendar", "pins", "uploads", "reactions", "gallery",
		)
	}

	seen := make(map[string]bool)
	all := make([]string, 0)
	for _, category := range selected {
		for _, tool := range catalog[category] {
			if !seen[tool] {
				seen[tool] = true
				all = append(all, tool)
			}
		}
	}
	sort.Strings(all)
	return all
}

// RegisterMetaTools registers MCP self-discovery/status tools.
func RegisterMetaTools(s *server.MCPServer, fallbackClient *client.Client) {
	listToolsTool := mcp.NewTool("groupme_list_available_tools",
		mcp.WithDescription("List GroupMe MCP tools available in the current TOOL_PROFILE. Use this before claiming no access to tools."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("profile",
			mcp.Description("Optional profile override: all, core, or messaging. Defaults to server TOOL_PROFILE."),
		),
	)

	s.AddTool(listToolsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := getArgs(request)
		profile := getFirstStringArg(args, "profile")
		if profile == "" {
			profile = os.Getenv("TOOL_PROFILE")
		}
		if profile == "" {
			profile = "all"
		}

		catalog := groupToolCatalog()
		result := map[string]interface{}{
			"tool_profile":           profile,
			"total_tools":            len(toolsForProfile(profile)),
			"available_tools":        toolsForProfile(profile),
			"categories":             catalog,
			"usage_hint":             "If a group or parent group ID is unknown, start with groupme_list_groups or groupme_list_available_tools. Subgroups are listed under /groups/{parent_group_id}/subgroups and read through /groups/{subgroup_id}/messages.",
			"integration_note":       "Some gateways rename display labels (for example token_* prefixes), but underlying GroupMe MCP tools are still available.",
			"recommended_first_step": "Call groupme_get_server_capabilities when authentication/access seems unclear.",
		}

		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})

	capabilitiesTool := mcp.NewTool("groupme_get_server_capabilities",
		mcp.WithDescription("Return transport/profile/auth capabilities and optionally verify current GroupMe authentication."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithBoolean("verify_auth",
			mcp.Description("If true (default), verifies token by calling /users/me."),
		),
	)

	s.AddTool(capabilitiesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := getArgs(request)
		verifyAuth := getBoolArg(args, "verify_auth", true)
		resolvedClient := client.Get(ctx, fallbackClient)

		transport := strings.TrimSpace(os.Getenv("MCP_TRANSPORT"))
		if transport == "" {
			transport = "http"
		}

		profile := strings.TrimSpace(os.Getenv("TOOL_PROFILE"))
		if profile == "" {
			profile = "all"
		}

		multiUserMode := os.Getenv("ENCRYPTION_KEY") != "" && os.Getenv("JWT_SECRET") != ""
		authConfigured := multiUserMode || os.Getenv("GROUPME_ACCESS_TOKEN") != ""

		result := map[string]interface{}{
			"mcp_transport":   transport,
			"tool_profile":    profile,
			"multi_user_mode": multiUserMode,
			"auth_configured": authConfigured,
			"can_list_tools":  true,
			"next_steps": []string{
				"Run groupme_list_available_tools to inspect callable tools.",
				"If auth is false, call groupme_login (multi-user) or configure GROUPME_ACCESS_TOKEN (single-user).",
			},
		}

		if verifyAuth {
			user, err := resolvedClient.GetCurrentUser(ctx)
			if err != nil {
				result["auth_verified"] = false
				result["auth_error"] = err.Error()
			} else {
				result["auth_verified"] = true
				result["current_user"] = map[string]string{
					"id":   user.ID,
					"name": user.Name,
				}
			}
		}

		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})

	probeAPITool := mcp.NewTool("groupme_probe_api_endpoints",
		mcp.WithDescription("Safely test one or more potential GroupMe API endpoints using read-only methods (GET, HEAD, OPTIONS). Returns per-attempt diagnostics, classification, JSON parse status, and notes to help investigate undocumented endpoints. For subgroup reads, prefer /groups/{subgroup_id}/messages over /conversations/{id}/messages."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("endpoint",
			mcp.Description("Single endpoint to probe, such as /groups/123/subgroups or /groups/456/messages?limit=5. Use this or candidates_json. For subgroup message reads, do not use /conversations/{id}/messages."),
		),
		mcp.WithString("method",
			mcp.Description("Optional HTTP method for the single endpoint. Allowed values: GET, HEAD, OPTIONS. Defaults to GET."),
		),
		mcp.WithString("label",
			mcp.Description("Optional label for the single endpoint attempt."),
		),
		mcp.WithString("candidates_json",
			mcp.Description("Optional JSON array of endpoint candidates to iterate through. Example: [{\"label\":\"subgroups-list\",\"method\":\"GET\",\"endpoint\":\"/groups/123/subgroups\"},{\"label\":\"subgroup-messages\",\"endpoint\":\"/groups/456/messages?limit=5\"}]"),
		),
		mcp.WithBoolean("stop_on_first_success",
			mcp.Description("If true, stops after the first successful probe. Defaults to false."),
		),
		mcp.WithBoolean("include_headers",
			mcp.Description("If true, includes response headers for each attempt."),
		),
		mcp.WithNumber("max_body_bytes",
			mcp.Description("Maximum response bytes to include per attempt before truncation. Defaults to 8192."),
		),
	)

	s.AddTool(probeAPITool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resolvedClient := client.Get(ctx, fallbackClient)
		result, err := probeAPIEndpointsHandler(ctx, resolvedClient, getArgs(request))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to probe endpoints: %v", err)), nil
		}

		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}
