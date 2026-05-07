package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/Quazmoz/groupme-mcp/internal/client"
)

var youtubeURLPattern = regexp.MustCompile(`https?://(?:www\.)?(?:youtube\.com/watch\?[^\s<>"']+|youtube\.com/shorts/[^\s<>"']+|youtu\.be/[^\s<>"']+)`)

var dedupeCache struct {
	sync.Mutex
	recent map[string]time.Time
}

func init() {
	dedupeCache.recent = make(map[string]time.Time)
}

// CheckHighImpact validates if high impact tools are enabled
func CheckHighImpact() error {
	if os.Getenv("HIGH_IMPACT_TOOLS_ENABLED") == "false" {
		return fmt.Errorf("high-impact tools are disabled by administrator (HIGH_IMPACT_TOOLS_ENABLED=false)")
	}
	if os.Getenv("DRY_RUN_HIGH_IMPACT_TOOLS") == "true" {
		return fmt.Errorf("dry-run mode is enabled; skipping high-impact action")
	}
	return nil
}

// CheckBroadcast validates if broadcast tools are enabled
func CheckBroadcast() error {
	if os.Getenv("BROADCAST_TOOLS_ENABLED") != "true" {
		return fmt.Errorf("broadcast tools are disabled by administrator (BROADCAST_TOOLS_ENABLED!=true)")
	}
	if os.Getenv("DRY_RUN_HIGH_IMPACT_TOOLS") == "true" {
		return fmt.Errorf("dry-run mode is enabled; skipping broadcast action")
	}
	return nil
}

// CheckMessageLength limits the size of text messages
func CheckMessageLength(text string) error {
	max := 1000
	if val := os.Getenv("MAX_MESSAGE_CHARS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			max = parsed
		}
	}
	if len(text) > max {
		return fmt.Errorf("message length %d exceeds max %d", len(text), max)
	}
	return nil
}

// CheckDedupe prevents identical messages sent repeatedly to the same target
func CheckDedupe(targetID, text string) error {
	if os.Getenv("DEDUPE_SENDS_ENABLED") == "false" {
		return nil
	}
	window := 60
	if val := os.Getenv("DEDUPE_WINDOW_SECONDS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			window = parsed
		}
	}
	
	key := targetID + ":" + text
	
	dedupeCache.Lock()
	defer dedupeCache.Unlock()
	
	if t, ok := dedupeCache.recent[key]; ok {
		if time.Since(t) < time.Duration(window)*time.Second {
			return fmt.Errorf("duplicate message suppressed (sent within last %d seconds)", window)
		}
	}
	
	dedupeCache.recent[key] = time.Now()
	
	for k, v := range dedupeCache.recent {
		if time.Since(v) > time.Duration(window)*time.Second {
			delete(dedupeCache.recent, k)
		}
	}
	return nil
}

// --- Helper Functions ---

func getString(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func getInt(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	if v, ok := args[key].(int); ok {
		return v
	}
	if v, ok := args[key].(int32); ok {
		return int(v)
	}
	if v, ok := args[key].(int64); ok {
		return int(v)
	}
	return defaultVal
}

// --- Group Handlers ---

type groupSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
	UpdatedAt   int64  `json:"updated_at,omitempty"`
	ShareURL    string `json:"share_url,omitempty"`
}

type groupSearchResponse struct {
	SearchedFor string         `json:"searched_for"`
	BestMatch   *groupSummary  `json:"best_match,omitempty"`
	MatchCount  int            `json:"match_count"`
	Matches     []groupSummary `json:"matches"`
	AI_Guidance string         `json:"ai_guidance,omitempty"`
}

func summarizeGroup(g client.Group) groupSummary {
	return groupSummary{
		ID:          g.ID,
		Name:        g.Name,
		Type:        g.Type,
		Description: g.Description,
		ImageURL:    g.ImageURL,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
		ShareURL:    g.ShareURL,
	}
}

func listGroupSummaries(ctx context.Context, c *client.Client) ([]groupSummary, error) {
	groups, err := c.ListAllGroupsWithOptions(ctx, true)
	if err != nil {
		return nil, err
	}

	summaries := make([]groupSummary, 0, len(groups))
	for _, g := range groups {
		summaries = append(summaries, summarizeGroup(g))
	}

	return summaries, nil
}

func searchGroupSummaries(ctx context.Context, c *client.Client, searchName string) (*groupSearchResponse, error) {
	groups, err := c.ListAllGroupsWithOptions(ctx, true)
	if err != nil {
		return nil, err
	}

	searchLower := strings.ToLower(strings.TrimSpace(searchName))
	matches := make([]groupSummary, 0)
	var bestMatch *groupSummary

	for _, g := range groups {
		nameLower := strings.ToLower(strings.TrimSpace(g.Name))
		if !strings.Contains(nameLower, searchLower) {
			continue
		}

		summary := summarizeGroup(g)
		if bestMatch == nil && nameLower == searchLower {
			s := summary
			bestMatch = &s
		}
		matches = append(matches, summary)
	}

	if bestMatch == nil && len(matches) > 0 {
		s := matches[0]
		bestMatch = &s
	}

	return &groupSearchResponse{
		SearchedFor: searchName,
		BestMatch:   bestMatch,
		MatchCount:  len(matches),
		Matches:     matches,
		AI_Guidance: "If you are looking for a sub-topic or channel within one of these groups, call 'groupme_get_group_subtopics' with the parent group ID. Subgroups are listed under /groups/{parent_group_id}/subgroups and their messages are read via /groups/{subgroup_id}/messages.",
	}, nil
}

func listGroupsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	return listGroupSummaries(ctx, c)
}

func searchGroupHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	name := getString(args, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return searchGroupSummaries(ctx, c, name)
}

func getGroupHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	return c.GetGroup(ctx, groupID)
}

type subtopicSummary struct {
	ID                     string   `json:"id"`
	ParentID               string   `json:"parent_id,omitempty"`
	Name                   string   `json:"name"`
	Description            string   `json:"description,omitempty"`
	Type                   string   `json:"type,omitempty"`
	MessageCount           int      `json:"message_count,omitempty"`
	LastMessageID          string   `json:"last_message_id,omitempty"`
	LastMessageCreatedAt   int64    `json:"last_message_created_at,omitempty"`
	LastMessageUpdatedAt   int64    `json:"last_message_updated_at,omitempty"`
	PreviewSender          string   `json:"preview_sender,omitempty"`
	PreviewText            string   `json:"preview_text,omitempty"`
	PreviewImageURL        string   `json:"preview_image_url,omitempty"`
	PreviewAttachmentTypes []string `json:"preview_attachment_types,omitempty"`
}

type subtopicLookupResult struct {
	GroupID           string             `json:"group_id"`
	Endpoint          string             `json:"endpoint"`
	RequestedName     string             `json:"requested_name,omitempty"`
	SubtopicsCount    int                `json:"subtopics_count"`
	Subtopics         []subtopicSummary  `json:"subtopics"`
	MatchedSubtopic   *subtopicSummary   `json:"matched_subtopic,omitempty"`
	AIGuidance        string             `json:"ai_guidance,omitempty"`
	DirectEndpoint    directLookupResult `json:"direct_endpoint"`
	MessageScanResult fallbackScanResult `json:"message_scan_fallback"`
}

type directLookupResult struct {
	Attempted    bool              `json:"attempted"`
	PagesFetched int               `json:"pages_fetched"`
	RawPages     []json.RawMessage `json:"raw_pages,omitempty"`
	Error        string            `json:"error,omitempty"`
}

type fallbackScanResult struct {
	Attempted       bool   `json:"attempted"`
	Used            bool   `json:"used"`
	MessagesScanned int    `json:"messages_scanned,omitempty"`
	Error           string `json:"error,omitempty"`
}

func normalizeSubgroupID(v interface{}) string {
	switch id := v.(type) {
	case string:
		return id
	case float64:
		return fmt.Sprintf("%.0f", id)
	default:
		return fmt.Sprintf("%v", id)
	}
}

func decodeSubgroupsRawPayload(raw []byte) ([]client.Subgroup, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return []client.Subgroup{}, nil
	}

	var envelope struct {
		Response  json.RawMessage `json:"response"`
		Subgroups json.RawMessage `json:"subgroups"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err == nil {
		for _, candidate := range []json.RawMessage{envelope.Response, envelope.Subgroups, envelope.Data} {
			candidate = json.RawMessage(bytes.TrimSpace(candidate))
			if len(candidate) == 0 || string(candidate) == "null" {
				continue
			}
			return decodeSubgroupsRawPayload(candidate)
		}
	}

	var subgroups []client.Subgroup
	if err := json.Unmarshal(trimmed, &subgroups); err == nil {
		return subgroups, nil
	}

	var wrapped struct {
		Subgroups []client.Subgroup `json:"subgroups"`
		Data      []client.Subgroup `json:"data"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err == nil {
		if wrapped.Subgroups != nil {
			return wrapped.Subgroups, nil
		}
		if wrapped.Data != nil {
			return wrapped.Data, nil
		}
	}

	return nil, fmt.Errorf("failed to decode subgroup payload")
}

func summarizeSubgroup(sg client.Subgroup) subtopicSummary {
	summary := subtopicSummary{
		ID:          normalizeSubgroupID(sg.ID),
		ParentID:    normalizeSubgroupID(sg.ParentID),
		Name:        sg.Topic,
		Description: sg.Description,
		Type:        sg.Type,
	}

	if sg.Messages == nil {
		return summary
	}

	summary.MessageCount = sg.Messages.Count
	summary.LastMessageID = sg.Messages.LastMessageID
	summary.LastMessageCreatedAt = sg.Messages.LastMessageCreatedAt
	summary.LastMessageUpdatedAt = sg.Messages.LastMessageUpdatedAt

	if sg.Messages.Preview == nil {
		return summary
	}

	summary.PreviewSender = sg.Messages.Preview.Nickname
	summary.PreviewText = sg.Messages.Preview.Text
	summary.PreviewImageURL = sg.Messages.Preview.ImageURL

	for _, attachment := range sg.Messages.Preview.Attachments {
		if attachment.Type == "" {
			continue
		}
		summary.PreviewAttachmentTypes = append(summary.PreviewAttachmentTypes, attachment.Type)
	}

	return summary
}

func buildSubtopicLookup(ctx context.Context, c *client.Client, groupID, requestedName string) (*subtopicLookupResult, error) {
	const perPage = client.DefaultPageSize

	result := &subtopicLookupResult{
		GroupID:       groupID,
		Endpoint:      fmt.Sprintf("/groups/%s/subgroups", groupID),
		RequestedName: requestedName,
		Subtopics:     []subtopicSummary{},
		AIGuidance:    "This tool returns subtopic metadata and only a last-message preview. Do not treat the preview as full history. Subgroups are listed under /groups/{parent_group_id}/subgroups; to inspect real subtopic messages or find links, call 'groupme_list_subtopic_messages' or 'groupme_get_latest_youtube_link_from_subtopic' with the matched subtopic ID, which is read via /groups/{subgroup_id}/messages.",
		DirectEndpoint: directLookupResult{
			Attempted: true,
		},
		MessageScanResult: fallbackScanResult{},
	}

	seen := make(map[string]bool)

	for page := 1; page <= client.MaxPaginationPages; page++ {
		rawPage, err := c.GetSubgroupsRaw(ctx, groupID, page, perPage)
		if err != nil {
			result.DirectEndpoint.Error = err.Error()
			break
		}

		result.DirectEndpoint.PagesFetched++
		if len(rawPage) > 0 {
			result.DirectEndpoint.RawPages = append(result.DirectEndpoint.RawPages, json.RawMessage(rawPage))
		}

		pageSubgroups, err := decodeSubgroupsRawPayload(rawPage)
		if err != nil {
			result.DirectEndpoint.Error = err.Error()
			break
		}

		for _, sg := range pageSubgroups {
			id := normalizeSubgroupID(sg.ID)
			if id == "" || seen[id] {
				continue
			}

			seen[id] = true
			result.Subtopics = append(result.Subtopics, summarizeSubgroup(sg))
		}

		if len(pageSubgroups) < perPage {
			break
		}
	}

	if len(result.Subtopics) == 0 {
		result.MessageScanResult.Attempted = true
		result.MessageScanResult.Used = true

		messages, err := c.ListAllMessages(ctx, groupID, 3000)
		if err != nil {
			result.MessageScanResult.Error = err.Error()
		} else {
			result.MessageScanResult.MessagesScanned = len(messages)
			for _, msg := range messages {
				if !msg.System || msg.Event == nil || msg.Event.Type != "group.subgroup_created" {
					continue
				}

				subgroupID, ok := msg.Event.Data["subgroup_id"].(string)
				if !ok || subgroupID == "" || seen[subgroupID] {
					continue
				}

				name, _ := msg.Event.Data["name"].(string)
				seen[subgroupID] = true
				result.Subtopics = append(result.Subtopics, subtopicSummary{
					ID:   subgroupID,
					Name: name,
				})
			}
		}
	}

	result.SubtopicsCount = len(result.Subtopics)

	if requestedName != "" {
		requestedLower := strings.ToLower(strings.TrimSpace(requestedName))
		for i := range result.Subtopics {
			if strings.ToLower(strings.TrimSpace(result.Subtopics[i].Name)) == requestedLower {
				result.MatchedSubtopic = &result.Subtopics[i]
				break
			}
		}

		if result.MatchedSubtopic == nil {
			for i := range result.Subtopics {
				if strings.Contains(strings.ToLower(result.Subtopics[i].Name), requestedLower) {
					result.MatchedSubtopic = &result.Subtopics[i]
					break
				}
			}
		}
	}

	return result, nil
}

func getGroupRawHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}

	raw, err := c.GetGroupRaw(ctx, groupID)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(raw), nil
}

func getSubtopicsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}

	return buildSubtopicLookup(ctx, c, groupID, getString(args, "subtopic_name"))
}

func createGroupHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	name := getString(args, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	description := getString(args, "description")
	return c.CreateGroup(ctx, name, description)
}

func updateGroupHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	name := getString(args, "name")
	description := getString(args, "description")
	imageURL := getString(args, "image_url")
	return c.UpdateGroup(ctx, groupID, name, description, imageURL)
}

func destroyGroupHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	return nil, c.DestroyGroup(ctx, groupID)
}

func listFormerGroupsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	return c.ListFormerGroups(ctx)
}

func rejoinGroupHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	return c.RejoinGroup(ctx, groupID)
}

func leaveGroupHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	membershipID := getString(args, "membership_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	// If membership_id not provided, try to get it from group
	if membershipID == "" {
		group, err := c.GetGroup(ctx, groupID)
		if err != nil {
			return nil, fmt.Errorf("failed to get group to determine membership_id: %v", err)
		}
		// Find current user's membership - get current user first
		user, err := c.GetCurrentUser(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get current user: %v", err)
		}
		for _, member := range group.Members {
			if member.UserID == user.ID {
				membershipID = member.ID
				break
			}
		}
		if membershipID == "" {
			return nil, fmt.Errorf("could not determine your membership_id in this group")
		}
	}
	return nil, c.LeaveGroup(ctx, groupID, membershipID)
}

func addMembersHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	membersJSON := getString(args, "members")
	var members []struct {
		Nickname    string `json:"nickname"`
		Email       string `json:"email,omitempty"`
		PhoneNumber string `json:"phone_number,omitempty"`
		UserID      string `json:"user_id,omitempty"`
	}
	if err := json.Unmarshal([]byte(membersJSON), &members); err != nil {
		return nil, fmt.Errorf("invalid members JSON: %w", err)
	}
	// Convert to client.MemberToAdd
	var memberReqs []client.MemberToAdd
	for _, m := range members {
		memberReqs = append(memberReqs, client.MemberToAdd{
			Nickname: m.Nickname,
			Email:    m.Email,
			Phone:    m.PhoneNumber,
			UserID:   m.UserID,
		})
	}
	return c.AddMembers(ctx, groupID, memberReqs)
}

func removeMemberHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	groupID := getString(args, "group_id")
	memberID := getString(args, "member_id")
	if groupID == "" || memberID == "" {
		return nil, fmt.Errorf("group_id and member_id are required")
	}
	return nil, c.RemoveMember(ctx, groupID, memberID)
}

func updateNicknameHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	groupID := getString(args, "group_id")
	nickname := getString(args, "nickname")
	if groupID == "" || nickname == "" {
		return nil, fmt.Errorf("group_id and nickname are required")
	}
	return c.UpdateNickname(ctx, groupID, nickname)
}

func listMembersHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupName := getString(args, "group_name")
	groupID := getString(args, "group_id")

	if groupID != "" {
		group, err := c.GetGroup(ctx, groupID)
		if err != nil {
			return nil, err
		}
		return group.Members, nil
	}
	if groupName != "" {
		group, err := c.SearchGroupByName(ctx, groupName)
		if err != nil {
			return nil, err
		}
		fullGroup, err := c.GetGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		return fullGroup.Members, nil
	}
	return nil, fmt.Errorf("group_name or group_id is required")
}

func whoIsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	name := getString(args, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	nameLower := strings.ToLower(name)

	type FoundIn struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		ID       string `json:"id"`
		Nickname string `json:"nickname,omitempty"`
		UserID   string `json:"user_id"`
	}
	var results []FoundIn

	// Search in groups
	groups, err := c.ListAllGroups(ctx)
	if err == nil {
		for _, group := range groups {
			for _, member := range group.Members {
				if strings.Contains(strings.ToLower(member.Nickname), nameLower) {
					results = append(results, FoundIn{
						Type:     "group",
						Name:     group.Name,
						ID:       group.ID,
						Nickname: member.Nickname,
						UserID:   member.UserID,
					})
				}
			}
		}
	}

	// Search in DM chats
	chats, err := c.ListChats(ctx, 1, 100)
	if err == nil {
		for _, chat := range chats {
			if strings.Contains(strings.ToLower(chat.OtherUser.Name), nameLower) {
				results = append(results, FoundIn{
					Type:   "dm",
					Name:   chat.OtherUser.Name,
					ID:     chat.OtherUser.ID,
					UserID: chat.OtherUser.ID,
				})
			}
		}
	}

	return map[string]interface{}{
		"searched_for": name,
		"found_count":  len(results),
		"results":      results,
	}, nil
}

func listMatchingGroupsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	name := getString(args, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	searchResult, err := searchGroupSummaries(ctx, c, name)
	if err != nil {
		return nil, err
	}
	return searchResult.Matches, nil
}

func sendToGroupByNameHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	groupName := getString(args, "group_name")
	text := getString(args, "text")
	if err := CheckMessageLength(text); err != nil {
		return nil, err
	}
	if err := CheckDedupe(groupName, text); err != nil {
		return nil, err
	}
	if groupName == "" || text == "" {
		return nil, fmt.Errorf("group_name and text are required")
	}
	group, err := c.SearchGroupByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	sourceGUID := uuid.New().String()
	return c.SendMessage(ctx, group.ID, text, sourceGUID)
}

func getGroupMessagesHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupName := getString(args, "group_name")
	if groupName == "" {
		groupName = getString(args, "name")
	}
	if groupName == "" {
		return nil, fmt.Errorf("group_name is required")
	}
	limit := getInt(args, "limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	group, err := c.SearchGroupByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	return c.ListMessages(ctx, group.ID, limit, "")
}

// --- Message Handlers ---

func listMessagesHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	beforeID := getString(args, "before_id")
	limit := getInt(args, "limit", 20)
	return c.ListMessages(ctx, groupID, limit, beforeID)
}

func sendMessageHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	groupID := getString(args, "group_id")
	text := getString(args, "text")
	if err := CheckMessageLength(text); err != nil {
		return nil, err
	}
	if err := CheckDedupe(groupID, text); err != nil {
		return nil, err
	}
	if groupID == "" || text == "" {
		return nil, fmt.Errorf("group_id and text are required")
	}
	sourceGUID := uuid.New().String()
	return c.SendMessage(ctx, groupID, text, sourceGUID)
}

func likeMessageHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	conversationID := getString(args, "conversation_id")
	messageID := getString(args, "message_id")
	if conversationID == "" || messageID == "" {
		return nil, fmt.Errorf("conversation_id and message_id are required")
	}
	return nil, c.LikeMessage(ctx, conversationID, messageID)
}

func unlikeMessageHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	conversationID := getString(args, "conversation_id")
	messageID := getString(args, "message_id")
	if conversationID == "" || messageID == "" {
		return nil, fmt.Errorf("conversation_id and message_id are required")
	}
	return nil, c.UnlikeMessage(ctx, conversationID, messageID)
}

func searchMessagesHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	query := getString(args, "query")
	if groupID == "" || query == "" {
		return nil, fmt.Errorf("group_id and query are required")
	}
	maxMessages := getInt(args, "max_messages", 50)
	return c.SearchMessagesInGroup(ctx, groupID, query, maxMessages)
}

type youtubeLinkCandidate struct {
	URL       string `json:"url"`
	MessageID string `json:"message_id,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Text      string `json:"text,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
}

type subtopicMessagesResponse struct {
	GroupID       string                              `json:"group_id"`
	SubgroupID    string                              `json:"subgroup_id"`
	SubtopicName  string                              `json:"subtopic_name,omitempty"`
	Subgroup      *client.Subgroup                    `json:"subgroup,omitempty"`
	SubgroupRaw   json.RawMessage                     `json:"subgroup_raw,omitempty"`
	MessageLookup *client.SubgroupMessageLookupResult `json:"message_lookup"`
	Warning       string                              `json:"warning,omitempty"`
}

type latestYouTubeLinkResult struct {
	GroupID        string                              `json:"group_id"`
	SubgroupID     string                              `json:"subgroup_id"`
	SubtopicName   string                              `json:"subtopic_name,omitempty"`
	LookbackHours  int                                 `json:"lookback_hours"`
	Lookup         *client.SubgroupMessageLookupResult `json:"message_lookup"`
	Subgroup       *client.Subgroup                    `json:"subgroup,omitempty"`
	SubgroupRaw    json.RawMessage                     `json:"subgroup_raw,omitempty"`
	LatestLink     *youtubeLinkCandidate               `json:"latest_link,omitempty"`
	AllLinkMatches []youtubeLinkCandidate              `json:"all_link_matches"`
	Warning        string                              `json:"warning,omitempty"`
}

func trimExtractedURL(raw string) string {
	return strings.TrimRight(raw, ".,!?;:)]}\"'")
}

func extractYouTubeURLsFromMessage(msg client.Message) []string {
	seen := make(map[string]bool)
	urls := make([]string, 0)

	for _, match := range youtubeURLPattern.FindAllString(msg.Text, -1) {
		clean := trimExtractedURL(match)
		if clean != "" && !seen[clean] {
			seen[clean] = true
			urls = append(urls, clean)
		}
	}

	for _, att := range msg.Attachments {
		clean := trimExtractedURL(att.URL)
		if clean == "" {
			continue
		}
		if youtubeURLPattern.MatchString(clean) && !seen[clean] {
			seen[clean] = true
			urls = append(urls, clean)
		}
	}

	return urls
}

func resolveSubgroup(ctx context.Context, c *client.Client, groupID, subgroupID, subtopicName string) (string, string, error) {
	if subgroupID != "" {
		if subtopicName != "" {
			return subgroupID, subtopicName, nil
		}
		if groupID == "" {
			return subgroupID, "", nil
		}
		sg, err := c.GetSubgroup(ctx, groupID, subgroupID)
		if err == nil {
			return subgroupID, sg.Topic, nil
		}
		return subgroupID, "", nil
	}

	if subtopicName == "" {
		return "", "", fmt.Errorf("either subgroup_id or subtopic_name is required")
	}

	lookup, err := buildSubtopicLookup(ctx, c, groupID, subtopicName)
	if err != nil {
		return "", "", err
	}
	if lookup.MatchedSubtopic == nil {
		return "", "", fmt.Errorf("no subtopic matched %q", subtopicName)
	}

	return lookup.MatchedSubtopic.ID, lookup.MatchedSubtopic.Name, nil
}

func getSubtopicMessagesResponse(ctx context.Context, c *client.Client, groupID, subgroupID, subtopicName string, limit int, beforeID string) (*subtopicMessagesResponse, error) {
	resolvedID, resolvedName, err := resolveSubgroup(ctx, c, groupID, subgroupID, subtopicName)
	if err != nil {
		return nil, err
	}

	resp := &subtopicMessagesResponse{
		GroupID:      groupID,
		SubgroupID:   resolvedID,
		SubtopicName: resolvedName,
	}

	if groupID != "" {
		subgroup, subgroupErr := c.GetSubgroup(ctx, groupID, resolvedID)
		if subgroupErr == nil {
			resp.Subgroup = subgroup
		}

		subgroupRaw, subgroupRawErr := c.GetSubgroupRaw(ctx, groupID, resolvedID)
		if subgroupRawErr == nil && len(subgroupRaw) > 0 {
			resp.SubgroupRaw = json.RawMessage(subgroupRaw)
		}
	}

	lookup, lookupErr := c.ListSubgroupMessagesBestEffort(ctx, groupID, resolvedID, limit, beforeID)
	resp.MessageLookup = lookup

	if lookupErr != nil && resp.Subgroup == nil && len(resp.SubgroupRaw) == 0 {
		return resp, lookupErr
	}

	if lookupErr != nil {
		resp.Warning = lookupErr.Error()
	}
	if lookup != nil && lookup.ScopeConfidence != "" && lookup.ScopeConfidence != "high" {
		if resp.Warning != "" {
			resp.Warning += " "
		}
		resp.Warning += "A fallback subgroup message endpoint succeeded. Verify the messages belong to the requested subtopic before acting on them."
	}

	return resp, nil
}

func getLatestYouTubeLinkFromSubtopic(ctx context.Context, c *client.Client, groupID, subgroupID, subtopicName string, lookbackHours, maxMessages int) (*latestYouTubeLinkResult, error) {
	if lookbackHours <= 0 {
		lookbackHours = 24
	}
	if maxMessages <= 0 {
		maxMessages = 100
	}

	msgResp, err := getSubtopicMessagesResponse(ctx, c, groupID, subgroupID, subtopicName, maxMessages, "")
	if err != nil {
		return nil, err
	}

	result := &latestYouTubeLinkResult{
		GroupID:        msgResp.GroupID,
		SubgroupID:     msgResp.SubgroupID,
		SubtopicName:   msgResp.SubtopicName,
		LookbackHours:  lookbackHours,
		Lookup:         msgResp.MessageLookup,
		Subgroup:       msgResp.Subgroup,
		SubgroupRaw:    msgResp.SubgroupRaw,
		AllLinkMatches: []youtubeLinkCandidate{},
		Warning:        msgResp.Warning,
	}

	cutoff := time.Now().Add(-time.Duration(lookbackHours) * time.Hour).Unix()
	seen := make(map[string]bool)

	if result.Lookup != nil {
		for _, msg := range result.Lookup.Messages {
			if msg.CreatedAt < cutoff {
				continue
			}

			for _, extracted := range extractYouTubeURLsFromMessage(msg) {
				key := fmt.Sprintf("%s|%s", extracted, msg.ID)
				if seen[key] {
					continue
				}
				seen[key] = true
				result.AllLinkMatches = append(result.AllLinkMatches, youtubeLinkCandidate{
					URL:       extracted,
					MessageID: msg.ID,
					Sender:    msg.Name,
					Text:      msg.Text,
					CreatedAt: msg.CreatedAt,
				})
			}
		}
	}

	if len(result.AllLinkMatches) == 0 && result.Subgroup != nil && result.Subgroup.Messages != nil && result.Subgroup.Messages.Preview != nil {
		previewText := result.Subgroup.Messages.Preview.Text
		for _, extracted := range youtubeURLPattern.FindAllString(previewText, -1) {
			clean := trimExtractedURL(extracted)
			if clean == "" {
				continue
			}
			result.AllLinkMatches = append(result.AllLinkMatches, youtubeLinkCandidate{
				URL:       clean,
				Sender:    result.Subgroup.Messages.Preview.Nickname,
				Text:      previewText,
				CreatedAt: result.Subgroup.Messages.LastMessageCreatedAt,
			})
		}
	}

	for i := range result.AllLinkMatches {
		if result.LatestLink == nil || result.AllLinkMatches[i].CreatedAt > result.LatestLink.CreatedAt {
			link := result.AllLinkMatches[i]
			result.LatestLink = &link
		}
	}

	if result.LatestLink == nil && result.Warning == "" {
		result.Warning = "No YouTube links were found in the available subtopic messages for the requested lookback window."
	}

	return result, nil
}

func listSubtopicMessagesHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	subgroupID := getString(args, "subgroup_id")
	if groupID == "" && subgroupID == "" {
		return nil, fmt.Errorf("group_id is required unless subgroup_id is provided")
	}

	return getSubtopicMessagesResponse(
		ctx,
		c,
		groupID,
		subgroupID,
		getString(args, "subtopic_name"),
		getInt(args, "limit", 20),
		getString(args, "before_id"),
	)
}

func getLatestYouTubeLinkFromSubtopicHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	subgroupID := getString(args, "subgroup_id")
	if groupID == "" && subgroupID == "" {
		return nil, fmt.Errorf("group_id is required unless subgroup_id is provided")
	}

	return getLatestYouTubeLinkFromSubtopic(
		ctx,
		c,
		groupID,
		subgroupID,
		getString(args, "subtopic_name"),
		getInt(args, "lookback_hours", 24),
		getInt(args, "max_messages", 100),
	)
}

func forwardLatestYouTubeLinkFromSubtopicHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	sourceGroupID := getString(args, "source_group_id")
	if sourceGroupID == "" {
		return nil, fmt.Errorf("source_group_id is required")
	}

	lookup, err := getLatestYouTubeLinkFromSubtopic(
		ctx,
		c,
		sourceGroupID,
		getString(args, "subgroup_id"),
		getString(args, "subtopic_name"),
		getInt(args, "lookback_hours", 24),
		getInt(args, "max_messages", 100),
	)
	if err != nil {
		return nil, err
	}
	if lookup.LatestLink == nil {
		return lookup, fmt.Errorf("no YouTube link found to forward")
	}

	allowLowConfidence := false
	if v, ok := args["allow_low_confidence"].(bool); ok {
		allowLowConfidence = v
	}
	if lookup.Lookup != nil && lookup.Lookup.ScopeConfidence != "" && lookup.Lookup.ScopeConfidence != "high" && !allowLowConfidence {
		return lookup, fmt.Errorf("refusing to forward because subgroup message scope came from a fallback endpoint; rerun with allow_low_confidence=true if you want to override")
	}

	targetGroupID := getString(args, "target_group_id")
	targetGroupName := getString(args, "target_group_name")
	if targetGroupID == "" && targetGroupName == "" {
		return nil, fmt.Errorf("target_group_id or target_group_name is required")
	}

	targetGroupLabel := targetGroupID
	if targetGroupID == "" {
		group, err := c.SearchGroupByName(ctx, targetGroupName)
		if err != nil {
			return nil, err
		}
		targetGroupID = group.ID
		targetGroupLabel = group.Name
	}

	text := lookup.LatestLink.URL
	if prefix := getString(args, "prefix_text"); prefix != "" {
		text = prefix + "\n" + text
	}

	sourceGUID := uuid.New().String()
	sent, err := c.SendMessage(ctx, targetGroupID, text, sourceGUID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"forwarded_link": lookup.LatestLink,
		"target_group": map[string]string{
			"id":   targetGroupID,
			"name": targetGroupLabel,
		},
		"sent_message":  sent,
		"source_lookup": lookup,
	}, nil
}

// --- DM Handlers ---

func listChatsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	page := getInt(args, "page", 1)
	perPage := getInt(args, "per_page", 20)
	return c.ListChats(ctx, page, perPage)
}

func listDMMessagesHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	otherUserID := getString(args, "other_user_id")
	if otherUserID == "" {
		return nil, fmt.Errorf("other_user_id is required")
	}
	beforeID := getString(args, "before_id")
	return c.ListDirectMessages(ctx, otherUserID, beforeID, "")
}

func sendDMHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	recipientID := getString(args, "recipient_id")
	text := getString(args, "text")
	if err := CheckMessageLength(text); err != nil {
		return nil, err
	}
	if err := CheckDedupe("dm_"+recipientID, text); err != nil {
		return nil, err
	}
	if recipientID == "" || text == "" {
		return nil, fmt.Errorf("recipient_id and text are required")
	}
	sourceGUID := uuid.New().String()
	return c.SendDirectMessage(ctx, recipientID, text, sourceGUID)
}

func getDMByNameHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	name := getString(args, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	chat, err := c.SearchChatByName(ctx, name)
	if err != nil {
		return nil, err
	}
	messages, err := c.ListDirectMessages(ctx, chat.OtherUser.ID, "", "")
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"other_user": chat.OtherUser,
		"messages":   messages,
	}, nil
}

func sendDMByNameHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	name := getString(args, "name")
	text := getString(args, "text")
	if err := CheckMessageLength(text); err != nil {
		return nil, err
	}
	if err := CheckDedupe("dm_"+name, text); err != nil {
		return nil, err
	}
	if name == "" || text == "" {
		return nil, fmt.Errorf("name and text are required")
	}
	chat, err := c.SearchChatByName(ctx, name)
	if err != nil {
		return nil, err
	}
	sourceGUID := uuid.New().String()
	return c.SendDirectMessage(ctx, chat.OtherUser.ID, text, sourceGUID)
}

// --- Bot Handlers ---

func listBotsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	return c.ListBots(ctx)
}

func createBotHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	name := getString(args, "name")
	groupID := getString(args, "group_id")
	callbackURL := getString(args, "callback_url")
	if name == "" || groupID == "" {
		return nil, fmt.Errorf("name and group_id are required")
	}
	return c.CreateBot(ctx, name, groupID, callbackURL)
}

func postBotMessageHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	botID := getString(args, "bot_id")
	text := getString(args, "text")
	if err := CheckMessageLength(text); err != nil {
		return nil, err
	}
	if err := CheckDedupe("bot_"+botID, text); err != nil {
		return nil, err
	}
	if botID == "" || text == "" {
		return nil, fmt.Errorf("bot_id and text are required")
	}
	return nil, c.PostBotMessage(ctx, botID, text)
}

func destroyBotHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	if err := CheckHighImpact(); err != nil {
		return nil, err
	}
	botID := getString(args, "bot_id")
	if botID == "" {
		return nil, fmt.Errorf("bot_id is required")
	}
	return nil, c.DestroyBot(ctx, botID)
}

// --- Poll Handlers ---

func listPollsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	if groupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	status := getString(args, "status")
	polls, err := c.ListPolls(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if status != "" && status != "all" {
		var filtered []client.Poll
		for _, poll := range polls {
			if strings.EqualFold(poll.Status, status) {
				filtered = append(filtered, poll)
			}
		}
		return filtered, nil
	}
	return polls, nil
}

func createPollHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	subject := getString(args, "subject")
	optionsStr := getString(args, "options")
	if groupID == "" || subject == "" || optionsStr == "" {
		return nil, fmt.Errorf("group_id, subject, and options are required")
	}

	var options []string
	if err := json.Unmarshal([]byte(optionsStr), &options); err != nil {
		options = strings.Split(optionsStr, ",")
		for i := range options {
			options[i] = strings.TrimSpace(options[i])
		}
	}

	if len(options) < 2 {
		return nil, fmt.Errorf("at least 2 options are required")
	}

	expiration := getInt(args, "expiration", 0)
	return c.CreatePoll(ctx, groupID, subject, options, expiration)
}

func getPollHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	groupID := getString(args, "group_id")
	pollID := getString(args, "poll_id")
	if groupID == "" || pollID == "" {
		return nil, fmt.Errorf("group_id and poll_id are required")
	}
	return c.GetPoll(ctx, groupID, pollID)
}

// --- User Handlers ---

func getCurrentUserHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	return c.GetCurrentUser(ctx)
}

func updateUserHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	name := getString(args, "name")
	email := getString(args, "email")
	zipCode := getString(args, "zip_code")
	if name == "" && email == "" && zipCode == "" {
		return nil, fmt.Errorf("at least one of name, email, or zip_code is required")
	}
	return c.UpdateUser(ctx, name, email, zipCode)
}

// --- Block Handlers ---

func listBlocksHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	userID := getString(args, "user_id")
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	return c.ListBlocks(ctx, userID)
}

func blockUserHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	userID := getString(args, "user_id")
	otherUserID := getString(args, "other_user_id")
	if userID == "" || otherUserID == "" {
		return nil, fmt.Errorf("user_id and other_user_id are required")
	}
	return c.BlockUser(ctx, userID, otherUserID)
}

func unblockUserHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	userID := getString(args, "user_id")
	otherUserID := getString(args, "other_user_id")
	if userID == "" || otherUserID == "" {
		return nil, fmt.Errorf("user_id and other_user_id are required")
	}
	return nil, c.UnblockUser(ctx, userID, otherUserID)
}

func blockExistsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	userID := getString(args, "user_id")
	otherUserID := getString(args, "other_user_id")
	if userID == "" || otherUserID == "" {
		return nil, fmt.Errorf("user_id and other_user_id are required")
	}
	exists, err := c.BlockExists(ctx, userID, otherUserID)
	if err != nil {
		return nil, err
	}
	return map[string]bool{"exists": exists}, nil
}
