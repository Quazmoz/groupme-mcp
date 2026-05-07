package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// DirectMessage represents a DM message.
type DirectMessage struct {
	ID             string        `json:"id"`
	SourceGUID     string        `json:"source_guid"`
	CreatedAt      int64         `json:"created_at"`
	UserID         string        `json:"user_id"`
	RecipientID    string        `json:"recipient_id"`
	SenderID       string        `json:"sender_id"`
	ConversationID string        `json:"conversation_id"`
	Name           string        `json:"name"`
	AvatarURL      string        `json:"avatar_url"`
	Text           string        `json:"text"`
	FavoritedBy    []string      `json:"favorited_by"`
	Attachments    []interface{} `json:"attachments"`
}

// Chat represents a direct message conversation.
type Chat struct {
	CreatedAt     int64 `json:"created_at"`
	UpdatedAt     int64 `json:"updated_at"`
	MessagesCount int   `json:"messages_count"`
	LastMessage   struct {
		ID             string `json:"id"`
		Text           string `json:"text"`
		SenderID       string `json:"sender_id"`
		RecipientID    string `json:"recipient_id"`
		CreatedAt      int64  `json:"created_at"`
		ConversationID string `json:"conversation_id"`
	} `json:"last_message"`
	OtherUser struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	} `json:"other_user"`
}

// ListChats returns direct message conversations.
func (c *Client) ListChats(ctx context.Context, page, perPage int) ([]Chat, error) {
	endpoint := fmt.Sprintf("/chats?page=%d&per_page=%d", page, perPage)
	data, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var chats []Chat
	if err := json.Unmarshal(data, &chats); err != nil {
		return nil, fmt.Errorf("failed to parse chats: %w", err)
	}

	return chats, nil
}

// ListDirectMessages returns messages from a DM conversation.
// This is a convenience wrapper - use ListDirectMessagesWithOptions for more control.
func (c *Client) ListDirectMessages(ctx context.Context, otherUserID string, beforeID string, sinceID string) ([]DirectMessage, error) {
	return c.ListDirectMessagesWithOptions(ctx, otherUserID, 20, beforeID, sinceID, "")
}

// ListDirectMessagesWithOptions returns messages from a DM conversation with full options.
// limit: Number of messages to return (default 20, max 100)
// beforeID: Returns messages before this ID
// sinceID: Returns most recent messages after this ID
// afterID: Returns messages immediately after this ID
func (c *Client) ListDirectMessagesWithOptions(ctx context.Context, otherUserID string, limit int, beforeID, sinceID, afterID string) ([]DirectMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	endpoint := fmt.Sprintf("/direct_messages?other_user_id=%s&limit=%d", otherUserID, limit)
	if beforeID != "" {
		endpoint += fmt.Sprintf("&before_id=%s", beforeID)
	}
	if sinceID != "" {
		endpoint += fmt.Sprintf("&since_id=%s", sinceID)
	}
	if afterID != "" {
		endpoint += fmt.Sprintf("&after_id=%s", afterID)
	}
	data, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	
	// Handle 304 Not Modified
	if data == nil {
		return []DirectMessage{}, nil
	}

	var result struct {
		Count    int             `json:"count"`
		Messages []DirectMessage `json:"direct_messages"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse DMs: %w", err)
	}

	return result.Messages, nil
}

// SendDirectMessage sends a direct message to a user.
func (c *Client) SendDirectMessage(ctx context.Context, recipientID, text, sourceGUID string) (*DirectMessage, error) {
	payload := fmt.Sprintf(`{"direct_message": {"source_guid": %q, "recipient_id": %q, "text": %q}}`, sourceGUID, recipientID, text)
	data, err := c.doRequest(ctx, "POST", "/direct_messages", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var result struct {
		Message DirectMessage `json:"direct_message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse DM: %w", err)
	}

	return &result.Message, nil
}

// SearchChatByName finds a DM chat by the other user's name.
func (c *Client) SearchChatByName(ctx context.Context, name string) (*Chat, error) {
	chats, err := c.ListChats(ctx, 1, 100)
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(name)
	for i, chat := range chats {
		if strings.Contains(strings.ToLower(chat.OtherUser.Name), nameLower) {
			return &chats[i], nil
		}
	}

	return nil, fmt.Errorf("no DM chat found with user matching '%s'", name)
}
