package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterCalendarTools registers calendar-related MCP tools.
func RegisterCalendarTools(s *server.MCPServer, c *client.Client) {
	// List events tool
	listEventsTool := mcp.NewTool("groupme_list_calendar_events",
		mcp.WithDescription("List upcoming calendar events for a group."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the group (e.g. '12345678'). DO NOT use the group name."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of events to retrieve (default 20)"),
		),
	)
	s.AddTool(listEventsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		limit := 20
		if l, ok := getArgs(request)["limit"].(float64); ok {
			limit = int(l)
		}

		// Use current time as default end_at (listing future events)
		// Or maybe don't filter? API says "end_at" is date before which no events listed.
		// So to get UPCOMING events, we should pass NOW? Or maybe not pass it at all?
		// Docs say: "end_at (required): string - an ISO 8601-formatted string which represents the date before which no events will be listed."
		// So if I want future events, I should pass NOW.
		now := time.Now().Format(time.RFC3339)

		events, err := c.ListCalendarEvents(ctx, groupID, now, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list events: %v", err)), nil
		}

		if len(events) == 0 {
			return mcp.NewToolResultText("No upcoming events found."), nil
		}

		result, err := json.MarshalIndent(events, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Create event tool
	createEventTool := mcp.NewTool("groupme_create_event",
		mcp.WithDescription("Create a calendar event in a group. Uses direct GroupMe event payload fields."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the group (e.g. '12345678'). DO NOT use the group name."),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name/title of the event. Alias accepted: title."),
		),
		mcp.WithString("description",
			mcp.Description("Description of the event (optional)"),
		),
		mcp.WithString("start_at",
			mcp.Required(),
			mcp.Description("Start time in ISO 8601 format (e.g., 2023-12-25T10:00:00-05:00)"),
		),
		mcp.WithString("end_at",
			mcp.Required(),
			mcp.Description("End time in ISO 8601 format (required)"),
		),
		mcp.WithString("timezone",
			mcp.Required(),
			mcp.Description("Timezone in TZ database format (e.g., 'America/New_York', 'America/Chicago', 'UTC')"),
		),
		mcp.WithBoolean("is_all_day",
			mcp.Description("Whether the event lasts all day (default false)"),
		),
	)
	s.AddTool(createEventTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)

		groupID := getFirstStringArg(args, "group_id", "conversation_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}
		name := getFirstStringArg(args, "name", "title")
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}
		startAt := getFirstStringArg(args, "start_at", "start")
		if startAt == "" {
			return mcp.NewToolResultError("start_at is required"), nil
		}
		endAt := getFirstStringArg(args, "end_at", "end")
		if endAt == "" {
			return mcp.NewToolResultError("end_at is required"), nil
		}
		timezone := getFirstStringArg(args, "timezone", "tz")
		if timezone == "" {
			return mcp.NewToolResultError("timezone is required (e.g., 'America/New_York', 'UTC')"), nil
		}

		description := getFirstStringArg(args, "description", "details")
		isAllDay := getBoolArg(args, "is_all_day", false)

		event, err := c.CreateEvent(ctx, groupID, name, description, startAt, endAt, timezone, isAllDay)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create event: %v", err)), nil
		}

		result, err := json.MarshalIndent(event, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})

	// Delete event tool
	deleteEventTool := mcp.NewTool("groupme_delete_event",
		mcp.WithDescription("Delete a calendar event."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The numeric ID of the group (e.g. '12345678'). DO NOT use the group name."),
		),
		mcp.WithString("event_id",
			mcp.Required(),
			mcp.Description("The ID of the event to delete"),
		),
	)
	s.AddTool(deleteEventTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		groupID, ok := getArgs(request)["group_id"].(string)
		if !ok || groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}
		eventID, ok := getArgs(request)["event_id"].(string)
		if !ok || eventID == "" {
			return mcp.NewToolResultError("event_id is required"), nil
		}

		if err := c.DeleteEvent(ctx, groupID, eventID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete event: %v", err)), nil
		}

		return mcp.NewToolResultText("Event deleted successfully"), nil
	})
}
