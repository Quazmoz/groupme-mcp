package tools

import (
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
