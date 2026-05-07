package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// Event represents a calendar event.
type Event struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	CreatorID   string         `json:"creator_id"`
	ChannelID   string         `json:"channel_id"` // group_id
	IsAllDay    bool           `json:"is_all_day"`
	Timezone    string         `json:"timezone"`
	StartAt     string         `json:"start_at"` // ISO 8601
	EndAt       string         `json:"end_at"`   // ISO 8601
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
	DeletedAt   string         `json:"deleted_at"`
	Going       []string       `json:"going"` // List of user IDs
	ImageURL    string         `json:"image_url,omitempty"`
	Location    *EventLocation `json:"location,omitempty"`
}

// EventLocation represents a location for a calendar event.
type EventLocation struct {
	Lat     string `json:"lat"`
	Lng     string `json:"lng"`
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

// ListCalendarEvents lists events for a group.
// endAt is ISO 8601 date (required) - events before this date will not be listed.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/conversations/calendar/
func (c *Client) ListCalendarEvents(ctx context.Context, groupID string, endAt string, limit int) ([]Event, error) {
	// end_at is required per API docs
	if endAt == "" {
		return nil, fmt.Errorf("end_at parameter is required")
	}
	url := fmt.Sprintf("/conversations/%s/events/list?end_at=%s&limit=%d", groupID, endAt, limit)

	data, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse events: %w", err)
	}
	return result.Events, nil
}

// CreateEvent creates a calendar event.
// This is a convenience wrapper - use CreateEventWithOptions for more control.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/conversations/calendar/
func (c *Client) CreateEvent(ctx context.Context, groupID, name, description, startAt, endAt, timezone string, isAllDay bool) (*Event, error) {
	return c.CreateEventWithOptions(ctx, groupID, name, description, startAt, endAt, timezone, isAllDay, "", nil, nil)
}

// CreateEventWithOptions creates a calendar event with all available options.
// imageURL: URL of an image for the event (optional)
// location: Location details for the event (optional)
// reminders: Array of reminder times in minutes before the event (optional)
func (c *Client) CreateEventWithOptions(ctx context.Context, groupID, name, description, startAt, endAt, timezone string, isAllDay bool, imageURL string, location *EventLocation, reminders []int) (*Event, error) {
	payload := map[string]interface{}{
		"name":        name,
		"description": description,
		"start_at":    startAt,
		"end_at":      endAt,
		"timezone":    timezone,
		"is_all_day":  isAllDay,
	}

	if imageURL != "" {
		payload["image_url"] = imageURL
	}
	if location != nil {
		payload["location"] = map[string]string{
			"lat":     location.Lat,
			"lng":     location.Lng,
			"name":    location.Name,
			"address": location.Address,
		}
	}
	if len(reminders) > 0 {
		payload["reminders"] = reminders
	}

	bodyLines, _ := json.Marshal(payload)
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/conversations/%s/events/create", groupID), bytes.NewReader(bodyLines))
	if err != nil {
		return nil, err
	}

	var wrapped struct {
		Event Event `json:"event"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Event.ID != "" {
		return &wrapped.Event, nil
	}

	var bare Event
	if err := json.Unmarshal(data, &bare); err == nil && (bare.ID != "" || bare.Name != "") {
		return &bare, nil
	}

	return nil, fmt.Errorf("failed to parse event response")
}

// DeleteEvent deletes a calendar event.
func (c *Client) DeleteEvent(ctx context.Context, groupID, eventID string) error {
	// Docs say DELETE /conversations/:group_id/events/delete?event_id=...
	url := fmt.Sprintf("/conversations/%s/events/delete?event_id=%s", groupID, eventID)
	_, err := c.doRequest(ctx, "DELETE", url, nil)
	return err
}
