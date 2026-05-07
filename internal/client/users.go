package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// User represents a GroupMe user.
type User struct {
	ID          string `json:"id"`
	PhoneNumber string `json:"phone_number"`
	ImageURL    string `json:"image_url"`
	Name        string `json:"name"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	Email       string `json:"email"`
	SMS         bool   `json:"sms"`
}

// GetCurrentUser returns the authenticated user.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	data, err := c.doRequest(ctx, "GET", EndpointGetCurrentUser(), nil)
	if err != nil {
		return nil, err
	}

	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to parse user: %w", err)
	}

	return &user, nil
}

// UpdateUser updates the authenticated user's profile.
// This is a convenience wrapper - use UpdateUserWithOptions for more control.
func (c *Client) UpdateUser(ctx context.Context, name, email, zipCode string) (*User, error) {
	return c.UpdateUserWithOptions(ctx, name, email, zipCode, "")
}

// UpdateUserWithOptions updates the authenticated user's profile with all available options.
// avatarURL: URL to a JPG/PNG/GIF image (will be converted to GroupMe's image service URL)
func (c *Client) UpdateUserWithOptions(ctx context.Context, name, email, zipCode, avatarURL string) (*User, error) {
	payloadMap := make(map[string]string)
	if name != "" {
		payloadMap["name"] = name
	}
	if email != "" {
		payloadMap["email"] = email
	}
	if zipCode != "" {
		payloadMap["zip_code"] = zipCode
	}
	if avatarURL != "" {
		payloadMap["avatar_url"] = avatarURL
	}
	payloadBytes, _ := json.Marshal(payloadMap)
	data, err := c.doRequest(ctx, "POST", "/users/update", strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, err
	}

	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to parse user: %w", err)
	}

	return &user, nil
}
