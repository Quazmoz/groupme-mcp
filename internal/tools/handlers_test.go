package tools

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

func TestBuildSubtopicLookupIncludesPreviewAndGuidance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups/90951330/subgroups" {
			t.Fatalf("unexpected path: %s", r.URL.RequestURI())
		}

		response := map[string]interface{}{
			"response": []map[string]interface{}{
				{
					"id":          109458263,
					"parent_id":   90951330,
					"topic":       "Online Mission",
					"description": "",
					"type":        "private",
					"messages": map[string]interface{}{
						"count":                   7620,
						"last_message_id":         "177401997782250602",
						"last_message_created_at": 1774019977,
						"last_message_updated_at": 1774019977,
						"preview": map[string]interface{}{
							"nickname":    "Samuel B",
							"text":        "12/113",
							"image_url":   "https://i.groupme.com/2048x1366.jpeg.example",
							"attachments": []map[string]interface{}{},
						},
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

	result, err := buildSubtopicLookup(context.Background(), c, "90951330", "Online Mission")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AIGuidance == "" {
		t.Fatalf("expected AI guidance to be populated")
	}
	if result.MatchedSubtopic == nil {
		t.Fatalf("expected a matched subtopic")
	}
	if result.MatchedSubtopic.MessageCount != 7620 {
		t.Fatalf("expected message count 7620, got %d", result.MatchedSubtopic.MessageCount)
	}
	if result.MatchedSubtopic.LastMessageID != "177401997782250602" {
		t.Fatalf("unexpected last message id: %s", result.MatchedSubtopic.LastMessageID)
	}
	if result.MatchedSubtopic.PreviewSender != "Samuel B" {
		t.Fatalf("unexpected preview sender: %s", result.MatchedSubtopic.PreviewSender)
	}
	if result.MatchedSubtopic.PreviewText != "12/113" {
		t.Fatalf("unexpected preview text: %s", result.MatchedSubtopic.PreviewText)
	}
}

func TestListSubtopicMessagesHandlerAcceptsSubgroupIDWithoutParentGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups/109458263/messages" {
			t.Fatalf("unexpected path: %s", r.URL.RequestURI())
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

	raw, err := listSubtopicMessagesHandler(context.Background(), c, map[string]interface{}{
		"subgroup_id": "109458263",
		"limit":       float64(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := raw.(*subtopicMessagesResponse)
	if !ok {
		t.Fatalf("unexpected result type: %T", raw)
	}
	if result.SubgroupID != "109458263" {
		t.Fatalf("unexpected subgroup id: %s", result.SubgroupID)
	}
	if result.GroupID != "" {
		t.Fatalf("expected empty parent group id, got %s", result.GroupID)
	}
	if result.MessageLookup == nil || len(result.MessageLookup.Messages) != 1 {
		t.Fatalf("expected one subgroup message, got %#v", result.MessageLookup)
	}
}

func TestProbeAPIEndpointsHandlerTriesCandidatesUntilSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/unknown":
			http.Error(w, `{"meta":{"code":404,"errors":["missing"]}}`, http.StatusNotFound)
		case "/groups/109458263/messages":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"response": map[string]interface{}{
					"count": 1,
					"messages": []map[string]interface{}{
						{
							"id":         "m1",
							"group_id":   "109458263",
							"name":       "Tester",
							"text":       "hello",
							"created_at": 1774019977,
						},
					},
				},
				"meta": map[string]interface{}{"code": 200},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL)

	raw, err := probeAPIEndpointsHandler(context.Background(), c, map[string]interface{}{
		"candidates_json":       `[{"label":"bad-guess","endpoint":"/unknown"},{"label":"good-guess","endpoint":"/groups/109458263/messages"}]`,
		"stop_on_first_success": true,
		"max_body_bytes":        float64(2048),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := raw.(*client.EndpointProbeResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", raw)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(result.Attempts))
	}
	if result.SuccessCount != 1 {
		t.Fatalf("expected exactly one success, got %d", result.SuccessCount)
	}
	if result.FirstSuccess == nil {
		t.Fatalf("expected first_success to be populated")
	}
	if result.FirstSuccess.Endpoint != "/groups/109458263/messages" {
		t.Fatalf("unexpected successful endpoint: %s", result.FirstSuccess.Endpoint)
	}
	if result.Attempts[0].HTTPStatus != http.StatusNotFound {
		t.Fatalf("expected first attempt status 404, got %d", result.Attempts[0].HTTPStatus)
	}
	if result.Attempts[0].Classification != "likely invalid path" {
		t.Fatalf("expected first attempt classification to be likely invalid path, got %q", result.Attempts[0].Classification)
	}
	if !result.Attempts[1].AuthAttached {
		t.Fatalf("expected auth_attached to be true for authenticated client")
	}
	if result.Attempts[1].RequestURL != server.URL+"/groups/109458263/messages" {
		t.Fatalf("unexpected request URL: %s", result.Attempts[1].RequestURL)
	}
	if !result.Attempts[1].JSONParseOK {
		t.Fatalf("expected successful JSON parse for valid JSON body")
	}
	if result.Attempts[1].Classification != "likely valid undocumented endpoint" {
		t.Fatalf("unexpected success classification: %q", result.Attempts[1].Classification)
	}
	if result.Attempts[1].EndpointFamily != "group_messages" {
		t.Fatalf("expected group_messages endpoint family, got %q", result.Attempts[1].EndpointFamily)
	}
	if !strings.Contains(result.Attempts[1].BodyPreview, `"messages"`) {
		t.Fatalf("expected successful probe body to include messages payload, got %s", result.Attempts[1].BodyPreview)
	}
}

func TestProbeAPIEndpointsHandlerRejectsNonGroupMeAbsoluteHosts(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)

	raw, err := probeAPIEndpointsHandler(context.Background(), c, map[string]interface{}{
		"endpoint": "https://example.com/private",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := raw.(*client.EndpointProbeResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", raw)
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(result.Attempts))
	}
	if result.Attempts[0].Success {
		t.Fatalf("expected blocked absolute host probe to fail")
	}
	if !strings.Contains(result.Attempts[0].LocalError, "not allowed") {
		t.Fatalf("expected host restriction error, got %q", result.Attempts[0].LocalError)
	}
}

func TestProbeAPIEndpointsHandlerPreservesBasePathAndQueryString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/groups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("expected query parameter to be preserved, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": map[string]interface{}{"ok": true},
			"meta":     map[string]interface{}{"code": 200},
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL + "/v3")

	raw, err := probeAPIEndpointsHandler(context.Background(), c, map[string]interface{}{
		"endpoint": "/groups?page=2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := raw.(*client.EndpointProbeResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", raw)
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(result.Attempts))
	}
	if result.Attempts[0].RequestURL != server.URL+"/v3/groups?page=2" {
		t.Fatalf("unexpected request URL: %s", result.Attempts[0].RequestURL)
	}
	if result.Attempts[0].UpstreamStatus != http.StatusOK {
		t.Fatalf("expected upstream status 200, got %d", result.Attempts[0].UpstreamStatus)
	}
}

func TestProbeAPIEndpointsHandlerClassifiesEmptyJSONSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/groups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": []interface{}{},
			"meta":     map[string]interface{}{"code": 200},
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL + "/v3")

	raw, err := probeAPIEndpointsHandler(context.Background(), c, map[string]interface{}{
		"endpoint": "/groups?page=1&per_page=5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := raw.(*client.EndpointProbeResult)
	attempt := result.Attempts[0]
	if !attempt.JSONParseOK {
		t.Fatalf("expected JSON parse success for empty JSON envelope")
	}
	if attempt.Classification != "likely valid but empty" {
		t.Fatalf("expected empty success classification, got %q", attempt.Classification)
	}
	if attempt.EndpointFamily != "list_groups" {
		t.Fatalf("expected list_groups endpoint family, got %q", attempt.EndpointFamily)
	}
}

func TestProbeAPIEndpointsHandlerClassifiesConversationMessagesAsUnsupportedForSubgroups(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/conversations/109458263/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"meta":{"code":404,"errors":["Not Found"]}}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL + "/v3")

	raw, err := probeAPIEndpointsHandler(context.Background(), c, map[string]interface{}{
		"endpoint": "/conversations/109458263/messages",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := raw.(*client.EndpointProbeResult)
	attempt := result.Attempts[0]
	if attempt.Classification != "likely unsupported resource" {
		t.Fatalf("expected unsupported-resource classification, got %q", attempt.Classification)
	}
	if attempt.EndpointFamily != "conversation_messages" {
		t.Fatalf("expected conversation_messages endpoint family, got %q", attempt.EndpointFamily)
	}
	if len(attempt.Notes) == 0 || !strings.Contains(strings.Join(attempt.Notes, " "), "/groups/{subgroup_id}/messages") {
		t.Fatalf("expected subgroup routing note, got %#v", attempt.Notes)
	}
}

func TestProbeAPIEndpointsHandlerPreservesHTML500Diagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/users/me" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<html><body>upstream exploded</body></html>"))
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL + "/v3")

	raw, err := probeAPIEndpointsHandler(context.Background(), c, map[string]interface{}{
		"endpoint": "/users/me",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := raw.(*client.EndpointProbeResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", raw)
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(result.Attempts))
	}

	attempt := result.Attempts[0]
	if attempt.LocalError != "" {
		t.Fatalf("expected no local error, got %q", attempt.LocalError)
	}
	if attempt.UpstreamStatus != http.StatusInternalServerError {
		t.Fatalf("expected upstream status 500, got %d", attempt.UpstreamStatus)
	}
	if !strings.Contains(attempt.ContentType, "text/html") {
		t.Fatalf("expected HTML content type, got %q", attempt.ContentType)
	}
	if attempt.JSONParseOK {
		t.Fatalf("expected HTML error page to fail JSON parsing")
	}
	if attempt.Classification != "inconclusive" {
		t.Fatalf("expected inconclusive classification for HTML 500, got %q", attempt.Classification)
	}
	if !strings.Contains(attempt.BodyPreview, "upstream exploded") {
		t.Fatalf("expected HTML body preview, got %q", attempt.BodyPreview)
	}
	if !strings.Contains(attempt.UpstreamError, "500") {
		t.Fatalf("expected upstream error to preserve status, got %q", attempt.UpstreamError)
	}
}

func TestProbeAPIEndpointsHandlerRedactsSecretsFromBodyPreview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/users/me" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("echo test-token from upstream"))
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL + "/v3")

	raw, err := probeAPIEndpointsHandler(context.Background(), c, map[string]interface{}{
		"endpoint": "/users/me",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := raw.(*client.EndpointProbeResult)
	attempt := result.Attempts[0]
	if strings.Contains(attempt.BodyPreview, "test-token") {
		t.Fatalf("expected token to be redacted from body preview, got %q", attempt.BodyPreview)
	}
	if !strings.Contains(attempt.BodyPreview, "[REDACTED]") {
		t.Fatalf("expected redaction marker in body preview, got %q", attempt.BodyPreview)
	}
}
