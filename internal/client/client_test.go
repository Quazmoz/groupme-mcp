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

// mockHandler is a helper to mock API responses
func mockHandler(t *testing.T, expectedMethod, expectedPath string, statusCode int, responseBody string, validateReq func(*http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != expectedMethod {
			t.Errorf("expected method %s, got %s", expectedMethod, r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		
		// Verify headers
		if r.Header.Get("X-Access-Token") == "" {
			t.Error("expected X-Access-Token header to be set")
		}
		
		if validateReq != nil {
			validateReq(r)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	}
}

func TestListGroups(t *testing.T) {
	// Setup mock server - API returns wrapped response
	response := `{"response": [
		{"id": "123", "name": "Test Group", "description": "A test group"}
	], "meta": {"code": 200}}`
	
	server := httptest.NewServer(mockHandler(t, "GET", "/groups", http.StatusOK, response, func(r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("expected page 1")
		}
	}))
	defer server.Close()

	// Setup client
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL)

	// Test
	groups, err := c.ListGroups(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}
	if groups[0].ID != "123" {
		t.Errorf("expected group ID 123, got %s", groups[0].ID)
	}
}

func TestSendMessage(t *testing.T) {
	// Setup mock server - API returns wrapped response
	server := httptest.NewServer(mockHandler(t, "POST", "/groups/123/messages", http.StatusOK, `{"response": {"message": {"id": "msg1", "text": "Hello"}}, "meta": {"code": 200}}`, func(r *http.Request) {
		var payload struct {
			Message struct {
				SourceGUID string `json:"source_guid"`
				Text       string `json:"text"`
			} `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Message.Text != "Hello" {
			t.Errorf("expected text Hello, got %s", payload.Message.Text)
		}
	}))
	defer server.Close()

	// Setup client
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL)

	// Test
	msg, err := c.SendMessage(context.Background(), "123", "Hello", "guid1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	if msg.Text != "Hello" {
		t.Errorf("expected message text Hello, got %s", msg.Text)
	}
}

func TestContextCancellation(t *testing.T) {
	// Setup mock server that sleeps
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // Wait for cancellation
	}))
	defer server.Close()

	// Setup client
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test
	_, err := c.ListGroups(ctx, 1, 10)
	if err == nil {
		t.Error("expected error due to context cancellation, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got: %v", err)
	}
}

func TestRateLimiting(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response": [], "meta": {"code": 200}}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL)

	// Should retry and succeed
	_, err := c.ListGroups(context.Background(), 1, 10)
	if err != nil {
		t.Errorf("unexpected error after retry: %v", err)
	}
	
	if attempts < 2 {
		t.Error("expected retries for rate limit")
	}
}

func TestServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := client.New("test-token", logger)
	c.SetBaseURL(server.URL)

	_, err := c.ListGroups(context.Background(), 1, 10)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestListPolls(t *testing.T) {
ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodGet {
t.Errorf("Expected method GET, got %s", r.Method)
}
// Correct endpoint is /poll/:group_id (not /poll/groups/:group_id)
if r.URL.Path != "/poll/123" {
t.Errorf("Expected path /poll/123, got %s", r.URL.Path)
}

response := map[string]interface{}{
"response": map[string]interface{}{
"polls": []map[string]interface{}{
{
"data": map[string]interface{}{
"id":      "poll1",
"subject": "Test Poll",
"options": []map[string]interface{}{
{"id": "opt1", "title": "Yes", "votes": 5},
},
},
},
},
},
"meta": map[string]interface{}{"code": 200},
}
json.NewEncoder(w).Encode(response)
}))
defer ts.Close()

c := client.New("test-token", nil)
c.SetBaseURL(ts.URL)

polls, err := c.ListPolls(context.Background(), "123")
if err != nil {
t.Fatalf("ListPolls failed: %v", err)
}

if len(polls) != 1 {
t.Errorf("Expected 1 poll, got %d", len(polls))
}
if polls[0].Subject != "Test Poll" {
t.Errorf("Expected poll subject 'Test Poll', got '%s'", polls[0].Subject)
}
}

func TestCreatePoll(t *testing.T) {
ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodPost {
t.Errorf("Expected method POST, got %s", r.Method)
}
// Correct endpoint is /poll/:group_id (not /poll/groups/:group_id)
if r.URL.Path != "/poll/123" {
t.Errorf("Expected path /poll/123, got %s", r.URL.Path)
}

var payload struct {
Subject string `json:"subject"`
Options []struct {
Title string `json:"title"`
} `json:"options"`
}
if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
t.Fatalf("Failed to decode request body: %v", err)
}

if payload.Subject != "New Poll" {
t.Errorf("Expected subject 'New Poll', got '%s'", payload.Subject)
}
if len(payload.Options) != 2 {
t.Errorf("Expected 2 options, got %d", len(payload.Options))
}

response := map[string]interface{}{
"response": map[string]interface{}{
"poll": map[string]interface{}{
"data": map[string]interface{}{
"id":      "poll_new",
"subject": "New Poll",
"options": []map[string]interface{}{
{"id": "1", "title": "A", "votes": 0},
{"id": "2", "title": "B", "votes": 0},
},
},
},
},
"meta": map[string]interface{}{"code": 200},
}
json.NewEncoder(w).Encode(response)
}))
defer ts.Close()

c := client.New("test-token", nil)
c.SetBaseURL(ts.URL)

poll, err := c.CreatePoll(context.Background(), "123", "New Poll", []string{"A", "B"}, 0)
if err != nil {
t.Fatalf("CreatePoll failed: %v", err)
}

if poll.ID != "poll_new" {
t.Errorf("Expected poll ID 'poll_new', got '%s'", poll.ID)
}
}

