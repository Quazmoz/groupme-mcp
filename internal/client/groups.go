package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Group represents a GroupMe group.
type Group struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Description   string   `json:"description"`
	ImageURL      string   `json:"image_url"`
	CreatorUserID string   `json:"creator_user_id"`
	CreatedAt     int64    `json:"created_at"`
	UpdatedAt     int64    `json:"updated_at"`
	ShareURL      string   `json:"share_url"`
	Members       []Member `json:"members"`
	Messages      struct {
		Count         int    `json:"count"`
		LastMessageID string `json:"last_message_id"`
		LastMessageAt int64  `json:"last_message_created_at"`
	} `json:"messages"`
}

// ListGroups returns the user's groups with pagination.
// Set omitMemberships to true to exclude member lists (significantly improves performance for large groups).
func (c *Client) ListGroups(ctx context.Context, page, perPage int) ([]Group, error) {
	return c.ListGroupsWithOptions(ctx, page, perPage, false)
}

// ListGroupsWithOptions returns the user's groups with pagination and options.
// Set omitMemberships to true to exclude member lists (significantly improves performance for large groups).
func (c *Client) ListGroupsWithOptions(ctx context.Context, page, perPage int, omitMemberships bool) ([]Group, error) {
	if perPage <= 0 {
		perPage = DefaultPageSize // Max allowed by API
	}
	if page <= 0 {
		page = 1
	}
	endpoint := EndpointListGroups(page, perPage, omitMemberships)
	data, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var groups []Group
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("failed to parse groups: %w", err)
	}

	return groups, nil
}

// ListAllGroups fetches all groups using auto-pagination.
func (c *Client) ListAllGroups(ctx context.Context) ([]Group, error) {
	return c.ListAllGroupsWithOptions(ctx, false)
}

// ListAllGroupsWithOptions fetches all groups using auto-pagination and options.
// Set omitMemberships to true to exclude member lists (significantly improves
// performance and reduces payload size for discovery/search flows).
func (c *Client) ListAllGroupsWithOptions(ctx context.Context, omitMemberships bool) ([]Group, error) {
	var allGroups []Group
	page := 1
	perPage := DefaultPageSize

	for {
		groups, err := c.ListGroupsWithOptions(ctx, page, perPage, omitMemberships)
		if err != nil {
			return nil, err
		}

		allGroups = append(allGroups, groups...)

		// If we got fewer than perPage results, we've reached the end
		if len(groups) < perPage {
			break
		}
		page++

		// Safety limit to prevent infinite loops
		if page > MaxPaginationPages {
			c.logger.Warn("Reached max pagination limit", "pages", MaxPaginationPages)
			break
		}
	}

	c.logger.Debug("Fetched total groups", "count", len(allGroups), "pages", page)
	return allGroups, nil
}

// GetGroup returns a specific group.
func (c *Client) GetGroup(ctx context.Context, groupID string) (*Group, error) {
	data, err := c.doRequest(ctx, "GET", EndpointGetGroup(groupID), nil)
	if err != nil {
		return nil, err
	}

	var group Group
	if err := json.Unmarshal(data, &group); err != nil {
		return nil, fmt.Errorf("failed to parse group: %w", err)
	}

	return &group, nil
}

// GetGroupRaw gets the completely raw, unfiltered JSON string from the GroupMe API for debugging missing fields.
func (c *Client) GetGroupRaw(ctx context.Context, groupID string) ([]byte, error) {
	return c.doRawRequest(ctx, "GET", EndpointGetGroup(groupID), nil)
}

// Subgroup represents a topic/subgroup returned by the /subgroups endpoint.
type Subgroup struct {
	ID            interface{}            `json:"id"`
	ParentID      interface{}            `json:"parent_id"`
	Topic         string                 `json:"topic"`
	Description   string                 `json:"description"`
	AvatarURL     string                 `json:"avatar_url"`
	Type          string                 `json:"type,omitempty"`
	CreatorUserID interface{}            `json:"creator_user_id,omitempty"`
	CreatedAt     int64                  `json:"created_at,omitempty"`
	UpdatedAt     int64                  `json:"updated_at,omitempty"`
	Messages      *SubgroupMessageWindow `json:"messages,omitempty"`
}

// GetSubgroups fetches the subgroups (topics/channels) of a group if supported by the undocumented API.
func (c *Client) GetSubgroups(ctx context.Context, groupID string) ([]Subgroup, error) {
	return c.ListSubgroups(ctx, groupID, 1, DefaultPageSize)
}

// ListSubgroups fetches one page of subgroups for a parent group.
func (c *Client) ListSubgroups(ctx context.Context, groupID string, page, perPage int) ([]Subgroup, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = DefaultPageSize
	}

	data, err := c.doRequest(ctx, "GET", EndpointListSubgroups(groupID, page, perPage), nil)
	if err != nil {
		return nil, err
	}

	var subgroups []Subgroup
	if err := json.Unmarshal(data, &subgroups); err != nil {
		return nil, fmt.Errorf("failed to parse subgroups: %w", err)
	}

	return subgroups, nil
}

// ListAllSubgroups fetches all subgroups using auto-pagination.
func (c *Client) ListAllSubgroups(ctx context.Context, groupID string) ([]Subgroup, error) {
	var allSubgroups []Subgroup
	page := 1
	perPage := DefaultPageSize

	for {
		subgroups, err := c.ListSubgroups(ctx, groupID, page, perPage)
		if err != nil {
			return nil, err
		}

		allSubgroups = append(allSubgroups, subgroups...)

		if len(subgroups) < perPage {
			break
		}

		page++
		if page > MaxPaginationPages {
			c.logger.Warn("Reached max subgroup pagination limit", "group_id", groupID, "pages", page-1)
			break
		}
	}

	return allSubgroups, nil
}

// GetSubgroupsRaw gets the full raw JSON data for a subgroup list page,
// including the GroupMe response envelope.
func (c *Client) GetSubgroupsRaw(ctx context.Context, groupID string, page, perPage int) ([]byte, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = DefaultPageSize
	}
	return c.doRawRequest(ctx, "GET", EndpointListSubgroups(groupID, page, perPage), nil)
}

// GetSubgroup fetches a single subgroup by ID.
func (c *Client) GetSubgroup(ctx context.Context, groupID, subgroupID string) (*Subgroup, error) {
	data, err := c.doRequest(ctx, "GET", EndpointGetSubgroup(groupID, subgroupID), nil)
	if err != nil {
		return nil, err
	}

	var subgroup Subgroup
	if err := json.Unmarshal(data, &subgroup); err != nil {
		return nil, fmt.Errorf("failed to parse subgroup: %w", err)
	}

	return &subgroup, nil
}

// GetSubgroupRaw returns the full raw API response for a single subgroup.
func (c *Client) GetSubgroupRaw(ctx context.Context, groupID, subgroupID string) ([]byte, error) {
	return c.doRawRequest(ctx, "GET", EndpointGetSubgroup(groupID, subgroupID), nil)
}

// CreateGroup creates a new group.
func (c *Client) CreateGroup(ctx context.Context, name, description string) (*Group, error) {
	payload := fmt.Sprintf(`{"name": %q, "description": %q}`, name, description)
	data, err := c.doRequest(ctx, "POST", "/groups", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var group Group
	if err := json.Unmarshal(data, &group); err != nil {
		return nil, fmt.Errorf("failed to parse group: %w", err)
	}

	return &group, nil
}

// UpdateGroup updates a group's name, description, or image.
func (c *Client) UpdateGroup(ctx context.Context, groupID, name, description, imageURL string) (*Group, error) {
	payloadMap := make(map[string]string)
	if name != "" {
		payloadMap["name"] = name
	}
	if description != "" {
		payloadMap["description"] = description
	}
	if imageURL != "" {
		payloadMap["image_url"] = imageURL
	}

	bodyLines, _ := json.Marshal(payloadMap)
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/update", groupID), bytes.NewReader(bodyLines))
	if err != nil {
		return nil, err
	}

	var group Group
	if err := json.Unmarshal(data, &group); err != nil {
		return nil, fmt.Errorf("failed to parse group: %w", err)
	}

	return &group, nil
}

// DestroyGroup deletes a group (creator only).
func (c *Client) DestroyGroup(ctx context.Context, groupID string) error {
	_, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/destroy", groupID), nil)
	return err
}

// RejoinGroup rejoins a group you previously left.
func (c *Client) RejoinGroup(ctx context.Context, groupID string) (*Group, error) {
	payload := fmt.Sprintf(`{"group_id": %q}`, groupID)
	data, err := c.doRequest(ctx, "POST", "/groups/join", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var group Group
	if err := json.Unmarshal(data, &group); err != nil {
		return nil, fmt.Errorf("failed to parse group: %w", err)
	}

	return &group, nil
}

// SearchGroupByName finds a group by name and returns it.
// Prefers exact matches over partial matches.
// Uses ListAllGroups to search across ALL groups, not just the first page.
func (c *Client) SearchGroupByName(ctx context.Context, name string) (*Group, error) {
	groups, err := c.ListAllGroups(ctx)
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(strings.TrimSpace(name))

	// First pass: look for exact match
	for i, g := range groups {
		if strings.ToLower(strings.TrimSpace(g.Name)) == nameLower {
			c.logger.Debug("SearchGroupByName: exact match found", "search", name, "result", g.Name)
			return &groups[i], nil
		}
	}

	// Second pass: look for partial match
	for i, g := range groups {
		if strings.Contains(strings.ToLower(g.Name), nameLower) {
			c.logger.Debug("SearchGroupByName: partial match found", "search", name, "result", g.Name)
			return &groups[i], nil
		}
	}

	return nil, fmt.Errorf("no group found matching '%s'", name)
}

// ValidateGroupName validates group name length.
func ValidateGroupName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("group name cannot be empty")
	}
	if len(name) > 140 {
		return fmt.Errorf("group name exceeds maximum length of 140 characters (got %d)", len(name))
	}
	return nil
}

// ListFormerGroups returns groups the user has left.
func (c *Client) ListFormerGroups(ctx context.Context) ([]Group, error) {
	data, err := c.doRequest(ctx, "GET", "/groups/former", nil)
	if err != nil {
		return nil, err
	}

	var groups []Group
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("failed to parse groups: %w", err)
	}

	return groups, nil
}

// CreateSubgroup creates a new subtopic/channel within a group.
// topic: name of the new subtopic
// description: optional description
// groupType: "private" or "announcement"
func (c *Client) CreateSubgroup(ctx context.Context, groupID, topic, description, groupType string) (*Subgroup, error) {
	if groupType == "" {
		groupType = "private"
	}
	payloadBytes, err := json.Marshal(map[string]string{
		"topic":       topic,
		"description": description,
		"group_type":  groupType,
	})
	if err != nil {
		return nil, err
	}

	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/subgroups", groupID), bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	var subgroup Subgroup
	if err := json.Unmarshal(data, &subgroup); err != nil {
		return nil, fmt.Errorf("failed to parse subgroup: %w", err)
	}
	return &subgroup, nil
}

// UpdateSubgroup updates a subtopic/channel within a group.
func (c *Client) UpdateSubgroup(ctx context.Context, groupID, subgroupID, topic, description, groupType string) (*Subgroup, error) {
	payload := make(map[string]string)
	if topic != "" {
		payload["topic"] = topic
	}
	if description != "" {
		payload["description"] = description
	}
	if groupType != "" {
		payload["group_type"] = groupType
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	data, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/groups/%s/subgroups/%s", groupID, subgroupID), bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	var subgroup Subgroup
	if err := json.Unmarshal(data, &subgroup); err != nil {
		return nil, fmt.Errorf("failed to parse subgroup: %w", err)
	}
	return &subgroup, nil
}

// DeleteSubgroup deletes a subtopic/channel.
func (c *Client) DeleteSubgroup(ctx context.Context, groupID, subgroupID string) error {
	_, err := c.doRequest(ctx, "DELETE", fmt.Sprintf("/groups/%s/subgroups/%s", groupID, subgroupID), nil)
	return err
}

// MuteSubgroup mutes a specific subtopic for a duration (in minutes). If duration is -1, it mutes forever.
func (c *Client) MuteSubgroup(ctx context.Context, groupID, subgroupID string, durationMinutes int) error {
	var payloadBytes []byte
	var err error

	if durationMinutes < 0 {
		payloadBytes = []byte(`{"duration": null}`)
	} else {
		payloadBytes = []byte(fmt.Sprintf(`{"duration": %d}`, durationMinutes))
	}

	_, err = c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/subgroups/%s/mute", groupID, subgroupID), bytes.NewReader(payloadBytes))
	return err
}

// UnmuteSubgroup unmutes a specific subtopic.
func (c *Client) UnmuteSubgroup(ctx context.Context, groupID, subgroupID string) error {
	_, err := c.doRequest(ctx, "POST", fmt.Sprintf("/groups/%s/subgroups/%s/unmute", groupID, subgroupID), nil)
	return err
}
