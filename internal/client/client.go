// Package client provides HTTP client for GroupMe API.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/url"
	"strings"

	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var globalRateLimiter struct {
	sync.Mutex
	requests []time.Time
}

func checkRateLimit() error {
	if os.Getenv("RATE_LIMIT_ENABLED") == "false" {
		return nil
	}
	maxReqs := 100
	window := 60

	if val := os.Getenv("RATE_LIMIT_REQUESTS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			maxReqs = parsed
		}
	}
	if val := os.Getenv("RATE_LIMIT_WINDOW_SECONDS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			window = parsed
		}
	}

	globalRateLimiter.Lock()
	defer globalRateLimiter.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Duration(window) * time.Second)

	var valid []time.Time
	for _, t := range globalRateLimiter.requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	globalRateLimiter.requests = valid

	if len(globalRateLimiter.requests) >= maxReqs {
		return fmt.Errorf("local rate limit exceeded: max %d requests per %d seconds", maxReqs, window)
	}

	globalRateLimiter.requests = append(globalRateLimiter.requests, now)
	return nil
}

const (
	defaultBaseURL  = "https://api.groupme.com/v3"
	DefaultPageSize = 100
	MaxRetries      = 3
	DefaultTimeout  = 30 * time.Second

	// Pagination limits
	MaxPaginationPages = 50
	MaxMessagesPerPage = 100
	MaxSearchMessages  = 500

	// HTTP client tuning
	MaxIdleConns        = 100
	MaxIdleConnsPerHost = 10
	IdleConnTimeout     = 90 * time.Second
)

type contextKey string

const clientContextKey contextKey = "groupme_client"

// NewContext returns a new context with the client attached.
func NewContext(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, clientContextKey, c)
}

// FromContext returns the client from the context if present, otherwise nil.
func FromContext(ctx context.Context) *Client {
	c, ok := ctx.Value(clientContextKey).(*Client)
	if !ok {
		return nil
	}
	return c
}

// Get returns the client from context, or the fallback if not found.
func Get(ctx context.Context, fallback *Client) *Client {
	if c := FromContext(ctx); c != nil {
		return c
	}
	return fallback
}

// Client is the GroupMe API client.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// New creates a new GroupMe API client.
func New(token string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	// Configure transport with connection pooling for better performance
	transport := &http.Transport{
		MaxIdleConns:        MaxIdleConns,
		MaxIdleConnsPerHost: MaxIdleConnsPerHost,
		IdleConnTimeout:     IdleConnTimeout,
	}

	return &Client{
		token:   token,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout:   DefaultTimeout,
			Transport: transport,
		},
		logger: logger,
	}
}

// SetBaseURL overrides the default API URL (useful for testing).
func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}

// logDebug prints debug messages using the structured logger.
func (c *Client) logDebug(msg string, args ...any) {
	c.logger.Debug(msg, args...)
}

// attachAccessTokenHeader sets the GroupMe auth header when a non-empty token
// is configured and reports whether auth was actually attached.
func (c *Client) attachAccessTokenHeader(req *http.Request) bool {
	token := strings.TrimSpace(c.token)
	if token == "" {
		return false
	}
	req.Header.Set("X-Access-Token", token)
	return true
}

func (c *Client) redactString(value string) string {
	token := strings.TrimSpace(c.token)
	if token == "" || value == "" {
		return value
	}
	return strings.ReplaceAll(value, token, "[REDACTED]")
}

func (c *Client) redactURLForLogs(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return c.redactString(rawURL)
	}

	if parsed.User != nil {
		username := parsed.User.Username()
		if username == "" {
			username = "[REDACTED]"
		}
		parsed.User = url.UserPassword(username, "[REDACTED]")
	}

	query := parsed.Query()
	for key, values := range query {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(lowerKey, "token") ||
			strings.Contains(lowerKey, "secret") ||
			strings.Contains(lowerKey, "key") ||
			strings.Contains(lowerKey, "auth") ||
			strings.Contains(lowerKey, "password") {
			for i := range values {
				values[i] = "[REDACTED]"
			}
			query[key] = values
		}
	}
	parsed.RawQuery = query.Encode()

	return c.redactString(parsed.String())
}

func buildURLFromBase(baseURL, endpoint string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid configured base URL: %w", err)
	}

	relative, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("invalid endpoint path: %w", err)
	}
	if relative.IsAbs() {
		return relative.String(), nil
	}

	baseCopy := *base
	basePath := strings.TrimRight(baseCopy.Path, "/")
	relativePath := strings.TrimLeft(relative.Path, "/")

	switch {
	case basePath == "" && relativePath == "":
		baseCopy.Path = "/"
	case basePath == "":
		baseCopy.Path = "/" + relativePath
	case relativePath == "":
		baseCopy.Path = basePath
	default:
		baseCopy.Path = basePath + "/" + relativePath
	}

	baseCopy.RawPath = ""
	baseCopy.RawQuery = relative.RawQuery
	baseCopy.Fragment = relative.Fragment

	return baseCopy.String(), nil
}

func (c *Client) buildAPIURL(endpoint string) (string, error) {
	return buildURLFromBase(c.baseURL, endpoint)
}

// Response wraps the API response.
type Response struct {
	Response json.RawMessage `json:"response"`
	Meta     struct {
		Code   int      `json:"code"`
		Errors []string `json:"errors"`
	} `json:"meta"`
}

// FormatTimestamp converts a Unix timestamp to a human-readable string.
func FormatTimestamp(unixTime int64) string {
	if unixTime == 0 {
		return ""
	}
	t := time.Unix(unixTime, 0)
	return t.Format("Jan 2, 2006 3:04 PM")
}

// FormatTimestampRelative returns a relative time string (e.g., "2 hours ago").
func FormatTimestampRelative(unixTime int64) string {
	if unixTime == 0 {
		return ""
	}
	t := time.Unix(unixTime, 0)
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}

// doRequest performs an HTTP request to the GroupMe API with rate limiting.
func (c *Client) doRequest(ctx context.Context, method, endpoint string, body io.Reader) (json.RawMessage, error) {
	return c.doRequestWithRetry(ctx, method, endpoint, body, 0)
}

// doRawRequest performs an HTTP request and returns the full raw response body.
// Unlike doRequest, this preserves the API envelope for debugging undocumented
// or partially supported endpoints.
func (c *Client) doRawRequest(ctx context.Context, method, endpoint string, body io.Reader) ([]byte, error) {
	return c.doRawRequestWithRetry(ctx, method, endpoint, body, 0)
}

// doRequestWithRetry handles rate limiting with exponential backoff.
func (c *Client) doRequestWithRetry(ctx context.Context, method, endpoint string, body io.Reader, attempt int) (json.RawMessage, error) {
	if err := checkRateLimit(); err != nil {
		return nil, err
	}
	reqURL, err := c.buildAPIURL(endpoint)
	if err != nil {
		return nil, err
	}

	// Log request in debug mode (without token)
	c.logger.Debug("API Request", "method", method, "endpoint", endpoint, "attempt", attempt+1)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add token as Header
	c.attachAccessTokenHeader(req)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle 304 Not Modified (no new messages) - this is a valid response, not an error
	if resp.StatusCode == 304 {
		c.logger.Debug("304 Not Modified - no new content")
		return nil, nil
	}

	// Handle rate limiting (429 Too Many Requests)
	if resp.StatusCode == 429 {
		if attempt < MaxRetries {
			// Exponential backoff: 1s, 2s, 4s
			waitTime := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			c.logger.Warn("Rate limited", "wait_time", waitTime, "attempt", attempt+1)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(waitTime):
				return c.doRequestWithRetry(ctx, method, endpoint, body, attempt+1)
			}
		}
		return nil, fmt.Errorf("rate limited: exceeded max retries (%d)", MaxRetries)
	}

	if resp.StatusCode >= 400 {
		c.logger.Error("API error", "status", resp.StatusCode, "body", string(respBody))

		// Provide helpful messages for known GroupMe API issues
		if resp.StatusCode == 500 {
			// Check for known problematic endpoints
			knownIssues := map[string]string{
				"/pinned/":       "The GroupMe pinned messages API is currently experiencing issues on GroupMe's servers. This is a known GroupMe bug, not an issue with this tool.",
				"/events/create": "The GroupMe calendar events API is currently experiencing issues on GroupMe's servers. This is a known GroupMe bug, not an issue with this tool.",
				"/events/list":   "The GroupMe calendar events API is currently experiencing issues on GroupMe's servers. This is a known GroupMe bug, not an issue with this tool.",
			}
			for pattern, msg := range knownIssues {
				if strings.Contains(endpoint, pattern) {
					return nil, fmt.Errorf("%s (HTTP 500)", msg)
				}
			}
			return nil, fmt.Errorf("GroupMe API server error (HTTP 500). This is typically a temporary issue on GroupMe's servers. Please try again later. Endpoint: %s", endpoint)
		}

		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Handle 2xx responses with empty body (e.g., bot posts return 202 with no body)
	if len(respBody) == 0 {
		c.logger.Debug("Empty response body (success)")
		return nil, nil
	}

	var apiResp Response
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Meta.Errors) > 0 {
		return nil, fmt.Errorf("API errors: %v", apiResp.Meta.Errors)
	}

	return apiResp.Response, nil
}

// doRawRequestWithRetry performs an HTTP request with retry logic and returns
// the full raw response body, including the GroupMe response envelope.
func (c *Client) doRawRequestWithRetry(ctx context.Context, method, endpoint string, body io.Reader, attempt int) ([]byte, error) {
	if err := checkRateLimit(); err != nil {
		return nil, err
	}
	reqURL, err := c.buildAPIURL(endpoint)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("API Raw Request", "method", method, "endpoint", endpoint, "attempt", attempt+1)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.attachAccessTokenHeader(req)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == 304 {
		c.logger.Debug("304 Not Modified - no new content")
		return nil, nil
	}

	if resp.StatusCode == 429 {
		if attempt < MaxRetries {
			waitTime := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			c.logger.Warn("Rate limited", "wait_time", waitTime, "attempt", attempt+1)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(waitTime):
				return c.doRawRequestWithRetry(ctx, method, endpoint, body, attempt+1)
			}
		}
		return nil, fmt.Errorf("rate limited: exceeded max retries (%d)", MaxRetries)
	}

	if resp.StatusCode >= 400 {
		c.logger.Error("API raw error", "status", resp.StatusCode, "body", string(respBody))

		if resp.StatusCode == 500 {
			knownIssues := map[string]string{
				"/pinned/":       "The GroupMe pinned messages API is currently experiencing issues on GroupMe's servers. This is a known GroupMe bug, not an issue with this tool.",
				"/events/create": "The GroupMe calendar events API is currently experiencing issues on GroupMe's servers. This is a known GroupMe bug, not an issue with this tool.",
				"/events/list":   "The GroupMe calendar events API is currently experiencing issues on GroupMe's servers. This is a known GroupMe bug, not an issue with this tool.",
			}
			for pattern, msg := range knownIssues {
				if strings.Contains(endpoint, pattern) {
					return nil, fmt.Errorf("%s (HTTP 500)", msg)
				}
			}
			return nil, fmt.Errorf("GroupMe API server error (HTTP 500). This is typically a temporary issue on GroupMe's servers. Please try again later. Endpoint: %s", endpoint)
		}

		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if len(respBody) == 0 {
		c.logger.Debug("Empty response body (success)")
		return nil, nil
	}

	var apiResp Response
	if err := json.Unmarshal(respBody, &apiResp); err == nil && len(apiResp.Meta.Errors) > 0 {
		return nil, fmt.Errorf("API errors: %v", apiResp.Meta.Errors)
	}

	return respBody, nil
}
