package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SubgroupMessagePreview represents the latest-message preview embedded in
// subgroup payloads.
type SubgroupMessagePreview struct {
	Nickname    string       `json:"nickname"`
	Text        string       `json:"text"`
	ImageURL    string       `json:"image_url"`
	Attachments []Attachment `json:"attachments"`
}

// SubgroupMessageWindow represents the message summary embedded in subgroup
// payloads returned by the undocumented subgroup endpoints.
type SubgroupMessageWindow struct {
	Count                int                     `json:"count"`
	LastMessageID        string                  `json:"last_message_id"`
	LastMessageCreatedAt int64                   `json:"last_message_created_at"`
	LastMessageUpdatedAt int64                   `json:"last_message_updated_at"`
	Preview              *SubgroupMessagePreview `json:"preview,omitempty"`
}

// MessageFetchAttempt records one endpoint attempt while trying to fetch
// subgroup-specific messages from undocumented APIs.
type MessageFetchAttempt struct {
	Endpoint    string          `json:"endpoint"`
	Success     bool            `json:"success"`
	Error       string          `json:"error,omitempty"`
	ParsedCount int             `json:"parsed_count,omitempty"`
	Raw         json.RawMessage `json:"raw,omitempty"`
}

// SubgroupMessageLookupResult contains the best-effort lookup result for
// subgroup messages across several undocumented endpoint candidates.
type SubgroupMessageLookupResult struct {
	GroupID         string                `json:"group_id"`
	SubgroupID      string                `json:"subgroup_id"`
	EndpointUsed    string                `json:"endpoint_used,omitempty"`
	ScopeConfidence string                `json:"scope_confidence,omitempty"`
	Attempts        []MessageFetchAttempt `json:"attempts"`
	Messages        []Message             `json:"messages"`
}

// Attachment represents a message attachment.
type Attachment struct {
	Type        string  `json:"type"`                  // image, location, split, emoji, file, video
	URL         string  `json:"url,omitempty"`         // for image, file, video
	Lat         string  `json:"lat,omitempty"`         // for location
	Lng         string  `json:"lng,omitempty"`         // for location
	Name        string  `json:"name,omitempty"`        // for location, file
	Placeholder string  `json:"placeholder,omitempty"` // for emoji
	CharMap     [][]int `json:"charmap,omitempty"`     // for emoji
	FileID      string  `json:"file_id,omitempty"`     // for file
}

// MessageEvent represents a system message event payload.
type MessageEvent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// Message represents a GroupMe message.
type Message struct {
	ID          string        `json:"id"`
	SourceGUID  string        `json:"source_guid"`
	CreatedAt   int64         `json:"created_at"`
	UserID      string        `json:"user_id"`
	GroupID     string        `json:"group_id"`
	Name        string        `json:"name"`
	AvatarURL   string        `json:"avatar_url"`
	Text        string        `json:"text"`
	System      bool          `json:"system"`
	FavoritedBy []string      `json:"favorited_by"`
	Attachments []Attachment  `json:"attachments"`
	Event       *MessageEvent `json:"event,omitempty"`
}

// ListMessages returns messages from a group.
// beforeID: Returns messages created before this message ID (for paging backwards)
// This is a convenience wrapper - use ListMessagesWithOptions for more control.
func (c *Client) ListMessages(ctx context.Context, groupID string, limit int, beforeID string) ([]Message, error) {
	return c.ListMessagesWithOptions(ctx, groupID, limit, beforeID, "", "")
}

// ListMessagesWithOptions returns messages from a group with full pagination options.
// beforeID: Returns messages created before this message ID (descending order)
// sinceID: Returns most recent messages created after this ID (may skip messages if >20 new)
// afterID: Returns messages immediately after this ID (ascending order)
// Only use one of beforeID, sinceID, or afterID at a time.
func (c *Client) ListMessagesWithOptions(ctx context.Context, groupID string, limit int, beforeID, sinceID, afterID string) ([]Message, error) {
	endpoint := EndpointGroupMessages(groupID, limit, beforeID, sinceID, afterID)
	data, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Handle 304 Not Modified (no messages found)
	if data == nil {
		return []Message{}, nil
	}

	return parseMessagesPayload(data)
}

func parseMessagesPayload(data []byte) ([]Message, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []Message{}, nil
	}

	var envelope Response
	if err := json.Unmarshal(trimmed, &envelope); err == nil && len(envelope.Response) > 0 {
		return parseMessagesPayload(envelope.Response)
	}

	var result struct {
		Count    int       `json:"count"`
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(trimmed, &result); err == nil && result.Messages != nil {
		return result.Messages, nil
	}

	var alt struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(trimmed, &alt); err == nil && alt.Messages != nil {
		return alt.Messages, nil
	}

	var bare []Message
	if err := json.Unmarshal(trimmed, &bare); err == nil {
		return bare, nil
	}

	return nil, fmt.Errorf("failed to parse messages payload")
}

func messagesMatchSubgroupScope(messages []Message, subgroupID string) bool {
	if subgroupID == "" || len(messages) == 0 {
		return true
	}

	matched := false
	for _, msg := range messages {
		if msg.GroupID == "" {
			continue
		}
		if msg.GroupID != subgroupID {
			return false
		}
		matched = true
	}

	return matched
}

// ListSubgroupMessagesBestEffort reads subgroup/subtopic messages using the
// validated groups namespace. Subgroup IDs currently behave like group IDs for
// message reads, so /groups/{subgroup_id}/messages is the canonical endpoint.
// The method still records attempts for diagnostics, but it no longer models
// subgroup reads as conversation-style traffic.
func (c *Client) ListSubgroupMessagesBestEffort(ctx context.Context, groupID, subgroupID string, limit int, beforeID string) (*SubgroupMessageLookupResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > MaxMessagesPerPage {
		limit = MaxMessagesPerPage
	}

	type endpointCandidate struct {
		Endpoint        string
		ScopeConfidence string
	}

	buildEndpoint := func(base string) string {
		if beforeID == "" {
			return fmt.Sprintf("%s?limit=%d", base, limit)
		}
		return fmt.Sprintf("%s?limit=%d&before_id=%s", base, limit, beforeID)
	}

	candidates := make([]endpointCandidate, 0)
	seenEndpoints := make(map[string]bool)
	addCandidate := func(endpoint, confidence string) {
		if endpoint == "" || seenEndpoints[endpoint] {
			return
		}
		seenEndpoints[endpoint] = true
		candidates = append(candidates, endpointCandidate{
			Endpoint:        endpoint,
			ScopeConfidence: confidence,
		})
	}

	// Current validated behavior: subgroup IDs are read through the groups
	// namespace and should be treated as addressable group-like resources.
	addCandidate(EndpointSubgroupMessages(subgroupID, limit, beforeID), "high")
	if groupID != "" {
		addCandidate(buildEndpoint(fmt.Sprintf("/groups/%s/subgroups/%s/messages", groupID, subgroupID)), "medium")
	}

	result := &SubgroupMessageLookupResult{
		GroupID:    groupID,
		SubgroupID: subgroupID,
		Attempts:   make([]MessageFetchAttempt, 0, len(candidates)),
		Messages:   []Message{},
	}

	for _, candidate := range candidates {
		attempt := MessageFetchAttempt{Endpoint: candidate.Endpoint}
		raw, err := c.doRawRequest(ctx, "GET", candidate.Endpoint, nil)
		if err != nil {
			attempt.Error = err.Error()
			result.Attempts = append(result.Attempts, attempt)
			continue
		}

		if len(raw) > 0 {
			attempt.Raw = json.RawMessage(raw)
		}

		messages, err := parseMessagesPayload(raw)
		if err != nil {
			attempt.Error = err.Error()
			result.Attempts = append(result.Attempts, attempt)
			continue
		}
		if !messagesMatchSubgroupScope(messages, subgroupID) {
			attempt.Error = fmt.Sprintf("parsed %d messages but they belong to a different group scope", len(messages))
			attempt.ParsedCount = len(messages)
			result.Attempts = append(result.Attempts, attempt)
			continue
		}

		attempt.Success = true
		attempt.ParsedCount = len(messages)
		result.Attempts = append(result.Attempts, attempt)
		result.EndpointUsed = candidate.Endpoint
		result.ScopeConfidence = candidate.ScopeConfidence
		result.Messages = messages
		return result, nil
	}

	return result, fmt.Errorf("failed to fetch subgroup messages from all known endpoint candidates")
}

// SendMessage sends a message to a group.
func (c *Client) SendMessage(ctx context.Context, groupID, text, sourceGUID string) (*Message, error) {
	payload := fmt.Sprintf(`{"message": {"source_guid": %q, "text": %q}}`, sourceGUID, text)
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/messages", groupID), strings.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var result struct {
		Message Message `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	return &result.Message, nil
}

// SendMessageWithMentions sends a message with @mentions to a group.
// The text should include the user names at the positions specified in mentions.
// For example: text="Hello @John" with mentions=[{UserID: "123", Start: 6, Length: 5}]
func (c *Client) SendMessageWithMentions(ctx context.Context, groupID, text, sourceGUID string, userIDs []string, loci [][]int) (*Message, error) {
	// Build the mentions attachment
	mentionAttachment := struct {
		Type    string   `json:"type"`
		UserIDs []string `json:"user_ids"`
		Loci    [][]int  `json:"loci"`
	}{
		Type:    "mentions",
		UserIDs: userIDs,
		Loci:    loci,
	}

	payload := struct {
		Message struct {
			SourceGUID  string        `json:"source_guid"`
			Text        string        `json:"text"`
			Attachments []interface{} `json:"attachments"`
		} `json:"message"`
	}{}
	payload.Message.SourceGUID = sourceGUID
	payload.Message.Text = text
	payload.Message.Attachments = []interface{}{mentionAttachment}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create payload: %w", err)
	}

	// Log the payload for debugging
	c.logger.Debug("SendMessageWithMentions payload", "payload", string(payloadBytes))
	c.logger.Debug("Mentioning info", "user_ids", userIDs, "loci", loci)

	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/messages", groupID), strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, err
	}

	var result struct {
		Message Message `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	return &result.Message, nil
}

// SendMessageWithImage sends a message with an image attachment.
func (c *Client) SendMessageWithImage(ctx context.Context, groupID, text, imageURL, sourceGUID string) (*Message, error) {
	imageAttachment := struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}{
		Type: "image",
		URL:  imageURL,
	}

	payload := struct {
		Message struct {
			SourceGUID  string        `json:"source_guid"`
			Text        string        `json:"text"`
			Attachments []interface{} `json:"attachments"`
		} `json:"message"`
	}{}
	payload.Message.SourceGUID = sourceGUID
	payload.Message.Text = text
	payload.Message.Attachments = []interface{}{imageAttachment}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create payload: %w", err)
	}

	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/messages", groupID), strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, err
	}

	var result struct {
		Message Message `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	return &result.Message, nil
}

// PinMessage pins a message in a conversation.
func (c *Client) PinMessage(ctx context.Context, conversationID, messageID string) error {
	url := fmt.Sprintf("/conversations/%s/messages/%s/pin", conversationID, messageID)
	_, err := c.doRequest(ctx, "POST", url, nil)
	return err
}

// UnpinMessage unpins a message in a conversation.
func (c *Client) UnpinMessage(ctx context.Context, conversationID, messageID string) error {
	url := fmt.Sprintf("/conversations/%s/messages/%s/unpin", conversationID, messageID)
	_, err := c.doRequest(ctx, "POST", url, nil)
	return err
}

// ListPinnedMessages lists pinned messages in a group.
func (c *Client) ListPinnedMessages(ctx context.Context, groupID string) ([]PinnedMessage, error) {
	url := fmt.Sprintf("/pinned/groups/%s/messages/", groupID)
	data, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Response might be wrapped or just array, docs show array example in some places but wrapped in others?
	// Trying simple array first as per most list endpoints, but docs were ambiguous.
	// Docs Sample: Status: 200 OK [ ... ]

	var messages []PinnedMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		// Try wrapped if direct array fails
		var wrapped struct {
			Messages []PinnedMessage `json:"messages"`
		}
		if err2 := json.Unmarshal(data, &wrapped); err2 == nil {
			return wrapped.Messages, nil
		}
		return nil, fmt.Errorf("failed to parse pinned messages: %w", err)
	}
	return messages, nil
}

// ReactToMessage adds a reaction (emoji) to a message.
func (c *Client) ReactToMessage(ctx context.Context, conversationID, messageID, emojiCode string) error {
	// emojiCode should be the actual unicode character, e.g., "👍"
	// API payload: { "like_icon": { "type": "unicode", "code": "👍" } }
	payload := map[string]interface{}{
		"like_icon": map[string]string{
			"type": "unicode",
			"code": emojiCode,
		},
	}

	bodyLines, _ := json.Marshal(payload)
	// Endpoint is /messages/:conversation_id/:message_id/like (yes, "like" handles reactions)
	url := fmt.Sprintf("/messages/%s/%s/like", conversationID, messageID)
	_, err := c.doRequest(ctx, "POST", url, bytes.NewReader(bodyLines))
	return err
}

// ListAllMessages fetches all messages from a group using auto-pagination.
func (c *Client) ListAllMessages(ctx context.Context, groupID string, maxMessages int) ([]Message, error) {
	if maxMessages <= 0 {
		maxMessages = 500 // Default limit
	}

	var allMessages []Message
	beforeID := ""

	for len(allMessages) < maxMessages {
		limit := 100
		if remaining := maxMessages - len(allMessages); remaining < 100 {
			limit = remaining
		}

		messages, err := c.ListMessages(ctx, groupID, limit, beforeID)
		if err != nil {
			return nil, err
		}

		if len(messages) == 0 {
			break
		}

		allMessages = append(allMessages, messages...)
		beforeID = messages[len(messages)-1].ID

		c.logger.Debug("Fetched messages", "batch_count", len(messages), "total_count", len(allMessages))
	}

	return allMessages, nil
}

// SearchMessagesInGroup searches for messages containing specific text.
func (c *Client) SearchMessagesInGroup(ctx context.Context, groupID, query string, maxMessages int) ([]Message, error) {
	if maxMessages <= 0 {
		maxMessages = 100
	}

	// Fetch messages to search through
	messages, err := c.ListAllMessages(ctx, groupID, maxMessages)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var matches []Message
	for _, msg := range messages {
		if strings.Contains(strings.ToLower(msg.Text), queryLower) {
			matches = append(matches, msg)
		}
	}

	c.logger.Debug("Searched messages", "query", query, "found", len(matches), "searched", len(messages))
	return matches, nil
}

// ValidateMessageText validates message text length.
func ValidateMessageText(text string) error {
	if len(text) == 0 {
		return fmt.Errorf("message text cannot be empty")
	}
	if len(text) > 1000 {
		return fmt.Errorf("message text exceeds maximum length of 1000 characters (got %d)", len(text))
	}
	return nil
}

// PinnedMessage represents a pinned message.
type PinnedMessage struct {
	Message
	PinnedAt string `json:"pinned_at"`
	PinnedBy string `json:"pinned_by"` // user_id
}

// Mention represents an @mention in a message.
type Mention struct {
	Type    string   `json:"type"` // always "mentions"
	UserIDs []string `json:"user_ids"`
	Loci    [][]int  `json:"loci"` // [[start, length], ...]
}

// SendMessageWithLocation sends a message with a location attachment.
func (c *Client) SendMessageWithLocation(ctx context.Context, groupID, text, sourceGUID string, lat, lng float64, locationName string) (*Message, error) {
	locationAttachment := struct {
		Type string `json:"type"`
		Lat  string `json:"lat"`
		Lng  string `json:"lng"`
		Name string `json:"name"`
	}{
		Type: "location",
		Lat:  fmt.Sprintf("%f", lat),
		Lng:  fmt.Sprintf("%f", lng),
		Name: locationName,
	}

	payload := struct {
		Message struct {
			SourceGUID  string        `json:"source_guid"`
			Text        string        `json:"text"`
			Attachments []interface{} `json:"attachments"`
		} `json:"message"`
	}{}
	payload.Message.SourceGUID = sourceGUID
	payload.Message.Text = text
	payload.Message.Attachments = []interface{}{locationAttachment}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create payload: %w", err)
	}

	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/messages", groupID), strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, err
	}

	var result struct {
		Message Message `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	return &result.Message, nil
}

// LikeMessage likes a message.
func (c *Client) LikeMessage(ctx context.Context, conversationID, messageID string) error {
	_, err := c.doRequest(ctx, "POST", fmt.Sprintf("/messages/%s/%s/like", conversationID, messageID), nil)
	return err
}

// UnlikeMessage unlikes a message.
func (c *Client) UnlikeMessage(ctx context.Context, conversationID, messageID string) error {
	_, err := c.doRequest(ctx, "POST", fmt.Sprintf("/messages/%s/%s/unlike", conversationID, messageID), nil)
	return err
}

// DeleteMessage deletes a message from a conversation.
// Note: You can only delete your own messages.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/groups/messages/
func (c *Client) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	_, err := c.doRequest(ctx, "DELETE", fmt.Sprintf("/conversations/%s/messages/%s", conversationID, messageID), nil)
	return err
}

// GetMessage retrieves a specific message by ID.
// Note: This uses the v4 API endpoint for direct message retrieval.
func (c *Client) GetMessage(ctx context.Context, groupID, messageID string) (*Message, error) {
	// Use v4 API for direct message retrieval
	// Build full URL since client defaults to v3
	v4URL := fmt.Sprintf("https://api.groupme.com/v4/groups/%s/messages/%s", groupID, messageID)

	req, err := http.NewRequestWithContext(ctx, "GET", v4URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Access-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Message Message `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	return &result.Message, nil
}
