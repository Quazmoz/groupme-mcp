package tools

import (
	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterCoreTools registers a minimal subset of the most important tools.
// This is designed for models with smaller context windows that cannot handle
// the full 72-tool catalog. It includes ~15 high-value tools that cover the
// most common use cases: reading groups, sending messages, DMs, and search.
func RegisterCoreTools(s *server.MCPServer, c *client.Client) {
	// We re-use the existing registrars but selectively register subsets.
	// Since the existing Register* functions register all tools in their category,
	// we call the full registrars for the most essential categories only.
	//
	// Core categories:
	// - Groups (includes list, search, send_to_group_by_name, get_group_messages, list_members)
	// - DMs (includes get_dm_by_name, send_dm_by_name)
	// - Users (includes get_current_user)
	// - Bots (includes list_bots)
	RegisterGroupTools(s, c)
	RegisterDMTools(s, c)
	RegisterUserTools(s, c)
	RegisterBotTools(s, c)
}

// RegisterMessagingTools registers messaging-focused tools (groups + messages + DMs).
func RegisterMessagingTools(s *server.MCPServer, c *client.Client) {
	RegisterGroupTools(s, c)
	RegisterMessageTools(s, c)
	RegisterDMTools(s, c)
	RegisterUserTools(s, c)
	RegisterReactionTools(s, c)
}
