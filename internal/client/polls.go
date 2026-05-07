package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// CreatePoll creates a new poll in a group.
// See: https://groupme-js.github.io/GroupMeCommunityDocs/api/groups/polls/
func (c *Client) CreatePoll(ctx context.Context, groupID, subject string, options []string, expiration int) (*Poll, error) {
	// Endpoint: POST /poll/:group_id
	endpoint := fmt.Sprintf("/poll/%s", groupID)

	// Build options array
	type CreatePollOption struct {
		Title string `json:"title"`
	}
	var pollOptions []CreatePollOption
	for _, opt := range options {
		pollOptions = append(pollOptions, CreatePollOption{Title: opt})
	}

	// Default expiration to 1 day (timestamp) if not set
	var expirationDate int64
	if expiration <= 0 {
		expirationDate = time.Now().Add(24 * time.Hour).Unix()
	} else {
		expirationDate = time.Now().Add(time.Duration(expiration) * time.Second).Unix()
	}

	// Payload match official docs
	payload := struct {
		Subject    string             `json:"subject"`
		Options    []CreatePollOption `json:"options"`
		Expiration int64              `json:"expiration"`
		Type       string             `json:"type"`       // "single" or "multi"
		Visibility string             `json:"visibility"` // "public" or "anonymous"
	}{
		Subject:    subject,
		Options:    pollOptions,
		Expiration: expirationDate,
		Type:       "multi",  // Default to allowing multiple votes
		Visibility: "public", // Default to public visibility
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Response structure: { "poll": { "data": Poll object, ... }, "message": ... }
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
