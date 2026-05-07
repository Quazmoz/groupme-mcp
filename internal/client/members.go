package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Member represents a group member.
type Member struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Muted    bool   `json:"muted"`
	ImageURL string `json:"image_url"`
}

// MemberToAdd represents a member to add to a group.
type MemberToAdd struct {
	Nickname string `json:"nickname"`
	UserID   string `json:"user_id,omitempty"`
	Phone    string `json:"phone_number,omitempty"`
	Email    string `json:"email,omitempty"`
}

// AddMembers adds members to a group.
func (c *Client) AddMembers(ctx context.Context, groupID string, members []MemberToAdd) (string, error) {
	payload := struct {
		Members []MemberToAdd `json:"members"`
	}{Members: members}
	payloadBytes, _ := json.Marshal(payload)

	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/members/add", groupID), strings.NewReader(string(payloadBytes)))
	if err != nil {
		return "", err
	}

	var result struct {
		ResultsID string `json:"results_id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return result.ResultsID, nil
}

// RemoveMember removes a member from a group.
func (c *Client) RemoveMember(ctx context.Context, groupID, membershipID string) error {
	_, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/members/%s/remove", groupID, membershipID), nil)
	return err
}

// UpdateNickname changes your nickname in a group.
func (c *Client) UpdateNickname(ctx context.Context, groupID, nickname string) (*Member, error) {
	payload := fmt.Sprintf(`{"membership": {"nickname": %q}}`, nickname)
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/memberships/update", groupID), strings.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var member Member
	if err := json.Unmarshal(data, &member); err != nil {
		return nil, fmt.Errorf("failed to parse member: %w", err)
	}

	return &member, nil
}

// UpdateMemberNickname changes another member's nickname in a group (requires admin).
func (c *Client) UpdateMemberNickname(ctx context.Context, groupID, membershipID, nickname string) (*Member, error) {
	payload := fmt.Sprintf(`{"membership": {"nickname": %q}}`, nickname)
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/memberships/%s", groupID, membershipID), strings.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var member Member
	if err := json.Unmarshal(data, &member); err != nil {
		return nil, fmt.Errorf("failed to parse member: %w", err)
	}

	return &member, nil
}

// LeaveGroup leaves a group.
func (c *Client) LeaveGroup(ctx context.Context, groupID, membershipID string) error {
	_, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/members/%s/remove", groupID, membershipID), nil)
	return err
}

// GetAddMembersResults gets the results of an add members request.
func (c *Client) GetAddMembersResults(ctx context.Context, groupID, resultsID string) ([]Member, error) {
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/groups/%s/members/results/%s", groupID, resultsID), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Members []Member `json:"members"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Members, nil
}

// ValidateNickname validates nickname length.
func ValidateNickname(nickname string) error {
	if len(nickname) == 0 {
		return fmt.Errorf("nickname cannot be empty")
	}
	if len(nickname) > 50 {
		return fmt.Errorf("nickname exceeds maximum length of 50 characters (got %d)", len(nickname))
	}
	return nil
}
