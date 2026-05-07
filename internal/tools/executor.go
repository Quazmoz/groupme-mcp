package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/Quazmoz/groupme-mcp/internal/client"
)

// ToolExecuteRequest is the JSON body for POST /tool/execute
type ToolExecuteRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolExecuteResponse is the JSON response for tool execution
type ToolExecuteResponse struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// ToolExecutor provides a REST handler for executing tools.
type ToolExecutor struct {
	logger         *slog.Logger
	fallbackClient *client.Client
	tools          map[string]ToolHandler
}

// ToolHandler is a function that executes a tool given a client and arguments.
type ToolHandler func(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error)

// NewToolExecutor creates a new ToolExecutor with all tools registered.
func NewToolExecutor(fallbackClient *client.Client, logger *slog.Logger) *ToolExecutor {
	e := &ToolExecutor{
		logger:         logger,
		fallbackClient: fallbackClient,
		tools:          make(map[string]ToolHandler),
	}
	e.registerAllTools()
	return e
}

// registerAllTools registers all tool handlers.
func (e *ToolExecutor) registerAllTools() {
	// Groups
	e.tools["groupme_list_groups"] = listGroupsHandler
	e.tools["groupme_search_group_by_name"] = searchGroupHandler
	e.tools["groupme_get_group"] = getGroupHandler
	e.tools["groupme_get_group_raw"] = getGroupRawHandler
	e.tools["groupme_get_group_subtopics"] = getSubtopicsHandler
	e.tools["groupme_create_group"] = createGroupHandler
	e.tools["groupme_update_group"] = updateGroupHandler
	e.tools["groupme_destroy_group"] = destroyGroupHandler
	e.tools["groupme_list_former_groups"] = listFormerGroupsHandler
	e.tools["groupme_rejoin_group"] = rejoinGroupHandler
	e.tools["groupme_leave_group"] = leaveGroupHandler
	e.tools["groupme_add_members"] = addMembersHandler
	e.tools["groupme_remove_member"] = removeMemberHandler
	e.tools["groupme_update_group_nickname"] = updateNicknameHandler
	e.tools["groupme_list_group_members"] = listMembersHandler
	e.tools["groupme_who_is"] = whoIsHandler
	e.tools["groupme_list_matching_groups"] = listMatchingGroupsHandler
	e.tools["groupme_send_to_group_by_name"] = sendToGroupByNameHandler
	e.tools["groupme_get_group_messages"] = getGroupMessagesHandler

	// Messages
	e.tools["groupme_list_messages"] = listMessagesHandler
	e.tools["groupme_list_subtopic_messages"] = listSubtopicMessagesHandler
	e.tools["groupme_get_latest_youtube_link_from_subtopic"] = getLatestYouTubeLinkFromSubtopicHandler
	e.tools["groupme_forward_latest_youtube_link_from_subtopic"] = forwardLatestYouTubeLinkFromSubtopicHandler
	e.tools["groupme_send_message"] = sendMessageHandler
	e.tools["groupme_like_message"] = likeMessageHandler
	e.tools["groupme_unlike_message"] = unlikeMessageHandler
	e.tools["groupme_search_messages"] = searchMessagesHandler

	// DMs
	e.tools["groupme_list_chats"] = listChatsHandler
	e.tools["groupme_list_dm_messages"] = listDMMessagesHandler
	e.tools["groupme_send_dm"] = sendDMHandler
	e.tools["groupme_get_dm_by_name"] = getDMByNameHandler
	e.tools["groupme_send_dm_by_name"] = sendDMByNameHandler

	// Bots
	e.tools["groupme_list_bots"] = listBotsHandler
	e.tools["groupme_create_bot"] = createBotHandler
	e.tools["groupme_post_bot_message"] = postBotMessageHandler
	e.tools["groupme_destroy_bot"] = destroyBotHandler

	// Polls
	e.tools["groupme_list_polls"] = listPollsHandler
	e.tools["groupme_create_poll"] = createPollHandler
	e.tools["groupme_get_poll"] = getPollHandler

	// Users
	e.tools["groupme_get_current_user"] = getCurrentUserHandler
	e.tools["groupme_update_user"] = updateUserHandler

	// Meta / exploration
	e.tools["groupme_probe_api_endpoints"] = probeAPIEndpointsHandler

	// Blocks
	e.tools["groupme_list_blocks"] = listBlocksHandler
	e.tools["groupme_block_user"] = blockUserHandler
	e.tools["groupme_unblock_user"] = unblockUserHandler
	e.tools["groupme_block_exists"] = blockExistsHandler
}

// HandleToolExecute is the HTTP handler for POST /tool/execute
func (e *ToolExecutor) HandleToolExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		e.writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req ToolExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		e.writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		e.writeError(w, "tool name is required", http.StatusBadRequest)
		return
	}

	// Get client: 1) from context (set by middleware), 2) from direct token header, 3) fallback
	ctx := r.Context()
	c := client.FromContext(ctx)
	if c != nil {
		e.logger.Info("Using client from context (middleware)")
	} else {
		// Check for direct token header (for non-middleware requests)
		if os.Getenv("DIRECT_TOKEN_HEADER_ENABLED") == "true" {
			if directToken := r.Header.Get("X-GroupMe-Access-Token"); directToken != "" {
				c = client.New(directToken, e.logger)
				e.logger.Info("Using per-request client from X-GroupMe-Access-Token")
			}
		}
		
		if c == nil {
			// Use fallback client
			c = e.fallbackClient
			e.logger.Warn("Using fallback client (no auth found in context or header)")
		}
	}

	if c == nil {
		e.writeError(w, "no authentication provided - please register your GroupMe token or provide X-GroupMe-Access-Token header", http.StatusUnauthorized)
		return
	}

	// Find and execute tool
	handler, ok := e.tools[req.Name]
	if !ok {
		e.writeError(w, fmt.Sprintf("unknown tool: %s", req.Name), http.StatusNotFound)
		return
	}

	result, err := handler(ctx, c, normalizeArgs(req.Arguments))
	if err != nil {
		e.writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	e.writeJSON(w, http.StatusOK, ToolExecuteResponse{Result: result})
}

func (e *ToolExecutor) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (e *ToolExecutor) writeError(w http.ResponseWriter, message string, code int) {
	e.writeJSON(w, code, ToolExecuteResponse{Error: message})
}
