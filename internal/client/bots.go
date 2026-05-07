package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Bot represents a GroupMe bot.
type Bot struct {
	BotID          string `json:"bot_id"`
	GroupID        string `json:"group_id"`
	Name           string `json:"name"`
	AvatarURL      string `json:"avatar_url"`
	CallbackURL    string `json:"callback_url"`
	DMNotification bool   `json:"dm_notification"`
}

// ListBots returns the user's bots.
func (c *Client) ListBots(ctx context.Context) ([]Bot, error) {
	data, err := c.doRequest(ctx, "GET", "/bots", nil)
	if err != nil {
		return nil, err
	}

	var bots []Bot
	if err := json.Unmarshal(data, &bots); err != nil {
		return nil, fmt.Errorf("failed to parse bots: %w", err)
	}

	return bots, nil
}

// CreateBot creates a new bot.
// This is a convenience wrapper - use CreateBotWithOptions for more control.
func (c *Client) CreateBot(ctx context.Context, name, groupID, callbackURL string) (*Bot, error) {
	return c.CreateBotWithOptions(ctx, name, groupID, callbackURL, "", false)
}

// CreateBotWithOptions creates a new bot with all available options.
// avatarURL: URL for the bot's avatar image (optional)
// dmNotification: If true, the bot will receive DM notifications (optional)
func (c *Client) CreateBotWithOptions(ctx context.Context, name, groupID, callbackURL, avatarURL string, dmNotification bool) (*Bot, error) {
	botPayload := map[string]interface{}{
		"name":     name,
		"group_id": groupID,
	}
	if callbackURL != "" {
		botPayload["callback_url"] = callbackURL
	}
	if avatarURL != "" {
		botPayload["avatar_url"] = avatarURL
	}
	if dmNotification {
		botPayload["dm_notification"] = true
	}

	payload := map[string]interface{}{"bot": botPayload}
	payloadBytes, _ := json.Marshal(payload)

	data, err := c.doRequest(ctx, "POST", "/bots", strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, err
	}

	var result struct {
		Bot Bot `json:"bot"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bot: %w", err)
	}

	return &result.Bot, nil
}

// PostBotMessage posts a message from a bot.
func (c *Client) PostBotMessage(ctx context.Context, botID, text string) error {
	payload := fmt.Sprintf(`{"bot_id": %q, "text": %q}`, botID, text)
	_, err := c.doRequest(ctx, "POST", "/bots/post", strings.NewReader(payload))
	return err
}

// DestroyBot deletes a bot.
func (c *Client) DestroyBot(ctx context.Context, botID string) error {
	payload := fmt.Sprintf(`{"bot_id": %q}`, botID)
	_, err := c.doRequest(ctx, "POST", "/bots/destroy", strings.NewReader(payload))
	return err
}
