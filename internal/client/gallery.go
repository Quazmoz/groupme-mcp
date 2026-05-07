package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListGallery lists messages with media in a conversation.
func (c *Client) ListGallery(ctx context.Context, conversationID string, limit int, beforeID string, acceptFiles bool) ([]Message, error) {
	url := fmt.Sprintf("/conversations/%s/gallery?limit=%d", conversationID, limit)
	if beforeID != "" {
		url += fmt.Sprintf("&before=%s", beforeID)
	}
	// "acceptFiles" defaults to 0 (false) in API if omitted.
	if acceptFiles {
		url += "&acceptFiles=1"
	}
	
	data, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse gallery: %w", err)
	}
	return result.Messages, nil
}
