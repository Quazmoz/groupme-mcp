package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PollOption represents an option in a poll
type PollOption struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Votes    int      `json:"votes,omitempty"`
	VoterIDs []string `json:"voter_ids,omitempty"` // Only in public polls
}

// Poll represents a GroupMe poll (the "data" portion of the API response)
type Poll struct {
	ID             string       `json:"id"`
	Subject        string       `json:"subject"`
	OwnerID        string       `json:"owner_id"`
	ConversationID string       `json:"conversation_id"`
	Status         string       `json:"status"` // "active", "past"
	CreatedAt      int64        `json:"created_at"`
	Expiration     int64        `json:"expiration"` // Unix timestamp in seconds
	LastModified   int64        `json:"last_modified"`
	Type           string       `json:"type"`       // "single", "multi"
	Visibility     string       `json:"visibility"` // "anonymous", "public"
	Options        []PollOption `json:"options"`
}

// PollWrapper wraps the poll data as returned by the API
type PollWrapper struct {
	Data      Poll     `json:"data"`
	UserVote  string   `json:"user_vote,omitempty"`  // For single votes
	UserVotes []string `json:"user_votes,omitempty"` // For multi votes
}

// ListPolls lists polls in a group.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/groups/polls/
func (c *Client) ListPolls(ctx context.Context, groupID string) ([]Poll, error) {
	// Endpoint: GET /poll/:group_id
	endpoint := fmt.Sprintf("/poll/%s", groupID)

	// Response contains array of poll wrappers with nested "data" field
	var response struct {
		Polls             []PollWrapper `json:"polls"`
		ContinuationToken *string       `json:"continuation_token"`
	}

	raw, err := c.doRequestWithRetry(ctx, http.MethodGet, endpoint, nil, 0)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract the data from each wrapper
	polls := make([]Poll, len(response.Polls))
	for i, wrapper := range response.Polls {
		polls[i] = wrapper.Data
	}

	return polls, nil
}

// CreatePollRequest contains options for creating a poll
type CreatePollRequest struct {
	GroupID        string
	Subject        string
	Options        []string
	ExpirationSecs int
	ExpirationUnix int64
	PollType       string
	Visibility     string
}

// CreatePollWithRequest creates a new poll with advanced options.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/groups/polls/
func (c *Client) CreatePollWithRequest(ctx context.Context, req CreatePollRequest) (*Poll, error) {
	if req.Subject == "" {
		return nil, fmt.Errorf("subject is required")
	}

	// Trim whitespace and filter out empty options
	var validOptions []string
	seen := make(map[string]bool)
	for _, opt := range req.Options {
		trimmed := strings.TrimSpace(opt)
		if trimmed == "" {
			continue
		}
		if !seen[trimmed] {
			seen[trimmed] = true
			validOptions = append(validOptions, trimmed)
		}
	}

	if len(validOptions) < 2 {
		return nil, fmt.Errorf("at least two unique, non-empty options are required")
	}

	pollType := req.PollType
	if pollType == "" {
		pollType = "multi" // Default backward compatibility
	} else if pollType != "single" && pollType != "multi" {
		return nil, fmt.Errorf("invalid poll type: %s", pollType)
	}

	visibility := req.Visibility
	if visibility == "" {
		visibility = "public"
	} else if visibility != "public" && visibility != "anonymous" {
		return nil, fmt.Errorf("invalid visibility: %s", visibility)
	}

	// Endpoint: POST /poll/:group_id
	endpoint := fmt.Sprintf("/poll/%s", req.GroupID)

	// Build options array
	type CreatePollOption struct {
		Title string `json:"title"`
	}
	var pollOptions []CreatePollOption
	for _, opt := range validOptions {
		pollOptions = append(pollOptions, CreatePollOption{Title: opt})
	}

	var expirationDate int64
	if req.ExpirationUnix > 0 {
		expirationDate = req.ExpirationUnix
	} else if req.ExpirationSecs > 0 {
		expirationDate = time.Now().Add(time.Duration(req.ExpirationSecs) * time.Second).Unix()
	} else {
		expirationDate = time.Now().Add(24 * time.Hour).Unix()
	}

	// Payload match official docs
	payload := struct {
		Subject    string             `json:"subject"`
		Options    []CreatePollOption `json:"options"`
		Expiration int64              `json:"expiration"`
		Type       string             `json:"type"`       // "single" or "multi"
		Visibility string             `json:"visibility"` // "public" or "anonymous"
	}{
		Subject:    req.Subject,
		Options:    pollOptions,
		Expiration: expirationDate,
		Type:       pollType,
		Visibility: visibility,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	var response struct {
		PollWrapper PollWrapper `json:"poll"`
	}

	raw, err := c.doRequestWithRetry(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body), 0)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response.PollWrapper.Data, nil
}

// CreatePoll creates a new poll in a group.
// Deprecated: use CreatePollWithRequest instead.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/groups/polls/
func (c *Client) CreatePoll(ctx context.Context, groupID, subject string, options []string, expiration int) (*Poll, error) {
	return c.CreatePollWithRequest(ctx, CreatePollRequest{
		GroupID:        groupID,
		Subject:        subject,
		Options:        options,
		ExpirationSecs: expiration,
		PollType:       "multi",
		Visibility:     "public",
	})
}

// GetPoll gets a specific poll.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/groups/polls/
func (c *Client) GetPoll(ctx context.Context, groupID, pollID string) (*Poll, error) {
	// Endpoint: GET /poll/:group_id/:poll_id
	endpoint := fmt.Sprintf("/poll/%s/%s", groupID, pollID)

	var response struct {
		PollWrapper PollWrapper `json:"poll"`
	}

	raw, err := c.doRequestWithRetry(ctx, http.MethodGet, endpoint, nil, 0)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response.PollWrapper.Data, nil
}

// VotePoll votes in a poll.
// For single-response polls, pass a single option ID.
// For multi-response polls, pass multiple option IDs.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/groups/polls/
func (c *Client) VotePoll(ctx context.Context, groupID, pollID string, optionIDs []string) (*Poll, error) {
	var endpoint string
	var body *bytes.Buffer

	if len(optionIDs) == 0 {
		return nil, fmt.Errorf("at least one option ID is required")
	}

	if len(optionIDs) == 1 {
		// Single-response poll: POST /poll/:group_id/:poll_id/:option_id
		endpoint = fmt.Sprintf("/poll/%s/%s/%s", groupID, pollID, optionIDs[0])
		body = nil
	} else {
		// Multi-response poll: POST /poll/:group_id/:poll_id/ with {"votes": [...]}
		endpoint = fmt.Sprintf("/poll/%s/%s", groupID, pollID)
		payload := struct {
			Votes []string `json:"votes"`
		}{
			Votes: optionIDs,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal vote payload: %w", err)
		}
		body = bytes.NewBuffer(data)
	}

	var response struct {
		PollWrapper PollWrapper `json:"poll"`
	}

	raw, err := c.doRequestWithRetry(ctx, http.MethodPost, endpoint, body, 0)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response.PollWrapper.Data, nil
}

// EndPoll ends a poll immediately.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/groups/polls/
func (c *Client) EndPoll(ctx context.Context, groupID, pollID string) (*Poll, error) {
	// Endpoint: POST /poll/:group_id/:poll_id/end
	endpoint := fmt.Sprintf("/poll/%s/%s/end", groupID, pollID)

	var response struct {
		PollWrapper PollWrapper `json:"poll"`
	}

	raw, err := c.doRequestWithRetry(ctx, http.MethodPost, endpoint, nil, 0)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response.PollWrapper.Data, nil
}
