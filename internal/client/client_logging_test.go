package client_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Quazmoz/groupme-mcp/internal/client"
)

func TestClientLogsAndReturnsSanitizedAPIErrors(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"sensitive-response-body","message":"do not expose","member":{"name":"Alice Example"}}`))
	}))
	defer server.Close()

	c := client.New("secret-token", logger)
	c.SetBaseURL(server.URL)

	_, err := c.ListGroups(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}

	if got := err.Error(); !strings.Contains(got, "HTTP 400") {
		t.Fatalf("expected safe HTTP status in error, got %q", got)
	}
	if got := err.Error(); !strings.Contains(got, "GET /groups?page=1&per_page=10") {
		t.Fatalf("expected method and endpoint in error, got %q", got)
	}
	for _, forbidden := range []string{
		"sensitive-response-body",
		"do not expose",
		"Alice Example",
		"secret-token",
		"Authorization",
	} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("error should not contain %q: %q", forbidden, err.Error())
		}
	}

	logOutput := logBuffer.String()
	for _, required := range []string{
		`"msg":"API error"`,
		`"status":400`,
		`"method":"GET"`,
		`"endpoint":"/groups?page=1&per_page=10"`,
		`"attempt":1`,
		`"content_type":"application/json"`,
	} {
		if !strings.Contains(logOutput, required) {
			t.Fatalf("expected log output to contain %q, got %q", required, logOutput)
		}
	}
	for _, forbidden := range []string{
		"sensitive-response-body",
		"do not expose",
		"Alice Example",
		"secret-token",
		"Authorization",
	} {
		if strings.Contains(logOutput, forbidden) {
			t.Fatalf("log output should not contain %q: %q", forbidden, logOutput)
		}
	}
}
