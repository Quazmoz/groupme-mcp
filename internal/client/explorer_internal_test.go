package client

import (
	"strings"
	"testing"
)

func TestBuildAPIURLPreservesBasePathAndQuery(t *testing.T) {
	c := New("secret-token", nil)
	c.SetBaseURL("https://api.groupme.com/v3")

	got, err := c.buildAPIURL("/groups?page=1&per_page=5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "https://api.groupme.com/v3/groups?page=1&per_page=5"
	if got != want {
		t.Fatalf("unexpected URL: got %q want %q", got, want)
	}
}

func TestRedactURLForLogsRemovesSecretsFromQuery(t *testing.T) {
	c := New("secret-token", nil)

	got := c.redactURLForLogs("https://api.groupme.com/v3/groups?access_token=secret-token&auth=abc123&page=1")
	if got == "" {
		t.Fatalf("expected redacted URL output")
	}
	if got == "https://api.groupme.com/v3/groups?access_token=secret-token&auth=abc123&page=1" {
		t.Fatalf("expected URL to be redacted, got %q", got)
	}
	if strings.Contains(got, "secret-token") || strings.Contains(got, "abc123") {
		t.Fatalf("expected secrets to be removed from URL, got %q", got)
	}
}
