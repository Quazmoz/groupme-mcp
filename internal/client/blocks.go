package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// Block represents a blocked user.
type Block struct {
	UserID        string `json:"user_id"`
	BlockedUserID string `json:"blocked_user_id"`
	CreatedAt     int64  `json:"created_at"`
}

// ListBlocks returns the user's blocked contacts.
func (c *Client) ListBlocks(ctx context.Context, userID string) ([]Block, error) {
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/blocks?user=%s", userID), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Blocks []Block `json:"blocks"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse blocks: %w", err)
	}

	return result.Blocks, nil
}

// BlockUser blocks another user.
func (c *Client) BlockUser(ctx context.Context, userID, otherUserID string) (*Block, error) {
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/blocks?user=%s&otherUser=%s", userID, otherUserID), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Block Block `json:"block"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse block: %w", err)
	}

	return &result.Block, nil
}

// UnblockUser unblocks a user.
func (c *Client) UnblockUser(ctx context.Context, userID, otherUserID string) error {
	_, err := c.doRequest(ctx, "DELETE", fmt.Sprintf("/blocks?user=%s&otherUser=%s", userID, otherUserID), nil)
	return err
}

// BlockExists checks if a block exists between two users.
func (c *Client) BlockExists(ctx context.Context, userID, otherUserID string) (bool, error) {
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/blocks/between?user=%s&otherUser=%s", userID, otherUserID), nil)
	if err != nil {
		return false, err
	}

	var result struct {
		Between bool `json:"between"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Between, nil
}
