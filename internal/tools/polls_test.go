package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestExtractAlnum(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Hello World", "helloworld"},
		{"Hello  World!", "helloworld"},
		{"emoji 😁 test", "emojitest"},
		{"123 ABC", "123abc"},
		{"  ", ""},
	}

	for _, c := range cases {
		actual := extractAlnum(c.input)
		if actual != c.expected {
			t.Errorf("extractAlnum(%q) = %q, expected %q", c.input, actual, c.expected)
		}
	}
}

func TestFindSubgroup(t *testing.T) {
	subgroups := []client.Subgroup{
		{ID: "1", Topic: "General Chat"},
		{ID: "2", Topic: "Food 🍔"},
		{ID: "3", Topic: " food "},
		{ID: "4", Topic: "Gaming"},
	}

	// Exact match
	sg, err := findSubgroup(subgroups, "General Chat")
	if err != nil || sg.ID != "1" {
		t.Errorf("expected General Chat match to return 1, got %v err %v", sg, err)
	}

	// Case-insensitive match
	sg, err = findSubgroup(subgroups, "general chat")
	if err != nil || sg.ID != "1" {
		t.Errorf("expected general chat match to return 1, got %v err %v", sg, err)
	}

	// Exact match even if ambiguous down the line
	sg, err = findSubgroup(subgroups, "Food 🍔")
	if err != nil || sg.ID != "2" {
		t.Errorf("expected exact match to prioritize 2, got %v err %v", sg, err)
	}

	// Ambiguity test - "food" matches both ID 2 and 3 via extractAlnum
	_, err = findSubgroup(subgroups, "f o o d")
	if err == nil {
		t.Errorf("expected error for ambiguous subgroup, got nil")
	}

	// Emoji insensitivity
	sg, err = findSubgroup(subgroups, "gaming 🎮")
	if err != nil || sg.ID != "4" {
		t.Errorf("expected emoji match to return 4, got %v err %v", sg, err)
	}

	// Emoji + whitespace normalization
	sg, err = findSubgroup(subgroups, "  Gaming  🎮  ")
	if err != nil || sg.ID != "4" {
		t.Errorf("expected whitespace/emoji match to return 4, got %v err %v", sg, err)
	}

	// No match
	_, err = findSubgroup(subgroups, "nothing")
	if err == nil {
		t.Errorf("expected error for missing subgroup, got nil")
	}
}

func TestParseCreatePollArgs(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"subject":         "Test",
				"options":         `["A", "B"]`,
				"expiration":      float64(3600),
				"expiration_unix": float64(1770000000),
				"poll_type":       "single",
				"visibility":      "anonymous",
			},
		},
	}

	parsed, err := parseCreatePollArgs(req)
	if err != nil {
		t.Fatalf("parseCreatePollArgs failed: %v", err)
	}

	if parsed.Subject != "Test" {
		t.Errorf("Expected subject Test, got %s", parsed.Subject)
	}
	if len(parsed.Options) != 2 || parsed.Options[0] != "A" {
		t.Errorf("Expected options [A, B], got %v", parsed.Options)
	}
	if parsed.ExpirationSecs != 3600 {
		t.Errorf("Expected exp 3600, got %d", parsed.ExpirationSecs)
	}
	if parsed.ExpirationUnix != 1770000000 {
		t.Errorf("Expected exp unix 1770000000, got %d", parsed.ExpirationUnix)
	}
	if parsed.PollType != "single" {
		t.Errorf("Expected poll type single, got %s", parsed.PollType)
	}
	if parsed.Visibility != "anonymous" {
		t.Errorf("Expected visibility anonymous, got %s", parsed.Visibility)
	}

	// Fallback to defaults
	reqDefault := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"subject": "Test",
				"options": `["A", "B"]`,
			},
		},
	}

	parsed, err = parseCreatePollArgs(reqDefault)
	if err != nil {
		t.Fatalf("parseCreatePollArgs failed: %v", err)
	}
	if parsed.PollType != "" { // default handled in CreatePollWithRequest
		t.Errorf("Expected empty poll type, got %s", parsed.PollType)
	}
}

func TestResolveGroupForWriteAmbiguity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"response":[
			{"id":"1","name":"Family Chat"},
			{"id":"2","name":"Family Planning"}
		],"meta":{"code":200}}`))
	}))
	defer server.Close()

	c := client.New("test-token", nil)
	c.SetBaseURL(server.URL)

	_, err := resolveGroupForWrite(context.Background(), c, "family")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), `multiple groups matched "family"`) {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Family Chat (ID: 1)") || !strings.Contains(err.Error(), "Family Planning (ID: 2)") {
		t.Fatalf("expected candidate list in error, got %v", err)
	}
}

func TestCreatePollByGroupNameAmbiguity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"response":[
			{"id":"1","name":"Family Chat"},
			{"id":"2","name":"Family Planning"}
		],"meta":{"code":200}}`))
	}))
	defer server.Close()

	c := client.New("test-token", nil)
	c.SetBaseURL(server.URL)

	_, _, err := createPollByGroupNameWithArgs(context.Background(), c, map[string]interface{}{
		"group_name": "family",
		"subject":    "Dinner?",
		"options":    `["Yes","No"]`,
	})
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), `multiple groups matched "family"`) {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestCreatePollInSubgroupByNameAmbiguity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"response":[
			{"id":"1","name":"Family Chat"},
			{"id":"2","name":"Family Planning"}
		],"meta":{"code":200}}`))
	}))
	defer server.Close()

	c := client.New("test-token", nil)
	c.SetBaseURL(server.URL)

	_, _, _, err := createPollInSubgroupByNameWithArgs(context.Background(), c, map[string]interface{}{
		"parent_group_name": "family",
		"subgroup_topic":    "Food",
		"subject":           "Dinner?",
		"options":           `["Yes","No"]`,
	})
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), `multiple groups matched "family"`) {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestPollCreationToolsRespectCheckHighImpact(t *testing.T) {
	t.Setenv("HIGH_IMPACT_TOOLS_ENABLED", "false")
	t.Setenv("DRY_RUN_HIGH_IMPACT_TOOLS", "false")

	c := client.New("test-token", nil)
	cases := []struct {
		name string
		run  func() error
	}{
		{
			name: "groupme_create_poll",
			run: func() error {
				_, err := createPollWithArgs(context.Background(), c, map[string]interface{}{
					"group_id": "123",
					"subject":  "Dinner?",
					"options":  `["Yes","No"]`,
				})
				return err
			},
		},
		{
			name: "groupme_create_poll_by_group_name",
			run: func() error {
				_, _, err := createPollByGroupNameWithArgs(context.Background(), c, map[string]interface{}{
					"group_name": "Family",
					"subject":    "Dinner?",
					"options":    `["Yes","No"]`,
				})
				return err
			},
		},
		{
			name: "groupme_create_poll_in_subgroup_by_name",
			run: func() error {
				_, _, _, err := createPollInSubgroupByNameWithArgs(context.Background(), c, map[string]interface{}{
					"parent_group_name": "Family",
					"subgroup_topic":    "Food",
					"subject":           "Dinner?",
					"options":           `["Yes","No"]`,
				})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected high-impact guard error")
			}
			if !strings.Contains(err.Error(), "HIGH_IMPACT_TOOLS_ENABLED=false") {
				t.Fatalf("expected high-impact guard error, got %v", err)
			}
		})
	}
}

func TestEndPollRespectsCheckHighImpact(t *testing.T) {
	t.Setenv("HIGH_IMPACT_TOOLS_ENABLED", "false")
	t.Setenv("DRY_RUN_HIGH_IMPACT_TOOLS", "false")

	c := client.New("test-token", nil)
	_, err := endPollWithArgs(context.Background(), c, map[string]interface{}{
		"group_id": "123",
		"poll_id":  "456",
	})
	if err == nil {
		t.Fatal("expected high-impact guard error")
	}
	if !strings.Contains(err.Error(), "HIGH_IMPACT_TOOLS_ENABLED=false") {
		t.Fatalf("expected high-impact guard error, got %v", err)
	}
}

func TestCreatePollByGroupNameSinglePartialMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/groups":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"response":[{"id":"42","name":"Family Chat"}],"meta":{"code":200}}`))
		case "/poll/42":
			var payload struct {
				Options []struct {
					Title string `json:"title"`
				} `json:"options"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("failed to decode poll payload: %v", err)
			}
			if len(payload.Options) != 2 {
				t.Fatalf("expected 2 options, got %d", len(payload.Options))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"response":{"poll":{"data":{"id":"poll1","subject":"Dinner?","options":[{"id":"1","title":"Yes"},{"id":"2","title":"No"}]}}},"meta":{"code":200}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := client.New("test-token", nil)
	c.SetBaseURL(server.URL)

	poll, group, err := createPollByGroupNameWithArgs(context.Background(), c, map[string]interface{}{
		"group_name": "family",
		"subject":    "Dinner?",
		"options":    `["Yes","No"]`,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if group.ID != "42" || poll.ID != "poll1" {
		t.Fatalf("unexpected result: group=%+v poll=%+v", group, poll)
	}
}
