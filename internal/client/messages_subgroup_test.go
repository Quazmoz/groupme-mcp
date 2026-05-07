package client_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Quazmoz/groupme-mcp/internal/client"
)

func TestListSubgroupMessagesBestEffortPrefersSubgroupGroupEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups/109458263/messages" {
			t.Fatalf("unexpected endpoint called: %s", r.URL.RequestURI())
		}

		response := map[string]interface{}{
			"response": map[string]interface{}{
				"count": 1,
				"messages": []map[string]interface{}{
					{
						"id":         "177401997782250602",
						"group_id":   "109458263",
						"name":       "Samuel B",
						"text":       "12/113",
						"created_at": 1774019977,
					},
				},
			},
			"meta": map[string]interface{}{"code": 200},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL)

	result, err := c.ListSubgroupMessagesBestEffort(context.Background(), "90951330", "109458263", 5, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EndpointUsed != "/groups/109458263/messages?limit=5" {
		t.Fatalf("expected subgroup endpoint, got %s", result.EndpointUsed)
	}
	if result.ScopeConfidence != "high" {
		t.Fatalf("expected high confidence, got %s", result.ScopeConfidence)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].GroupID != "109458263" {
		t.Fatalf("expected subgroup-scoped group_id, got %s", result.Messages[0].GroupID)
	}
}

func TestListSubgroupMessagesBestEffortRejectsParentGroupFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/groups/109458263/messages":
			response := map[string]interface{}{
				"response": map[string]interface{}{
					"count": 1,
					"messages": []map[string]interface{}{
						{
							"id":         "177397563090876008",
							"group_id":   "90951330",
							"name":       "My Daily Bot",
							"text":       "Parent group message",
							"created_at": 1773975630,
						},
					},
				},
				"meta": map[string]interface{}{"code": 200},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		case r.URL.Path == "/groups/90951330/subgroups/109458263/messages":
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		default:
			t.Fatalf("unexpected endpoint called: %s", r.URL.RequestURI())
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL)

	result, err := c.ListSubgroupMessagesBestEffort(context.Background(), "90951330", "109458263", 5, "")
	if err == nil {
		t.Fatalf("expected subgroup lookup to fail when only parent-group messages are returned")
	}
	if !strings.Contains(err.Error(), "failed to fetch subgroup messages") {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(result.Attempts))
	}

	scopeRejected := false
	for _, attempt := range result.Attempts {
		if strings.Contains(attempt.Endpoint, "/conversations/") {
			t.Fatalf("conversation-based subgroup endpoint should not be attempted anymore: %s", attempt.Endpoint)
		}
		if strings.Contains(attempt.Error, "different group scope") {
			scopeRejected = true
			break
		}
	}
	if !scopeRejected {
		t.Fatalf("expected a scope-rejection attempt, got %#v", result.Attempts)
	}
}
