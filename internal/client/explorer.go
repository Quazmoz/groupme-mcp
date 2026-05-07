package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultProbeMaxBodyBytes = 8192
	MaxProbeCandidates       = 20
)

var allowedProbeMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

var allowedProbeHosts = map[string]bool{
	"api.groupme.com":   true,
	"image.groupme.com": true,
	"file.groupme.com":  true,
	"video.groupme.com": true,
}

// EndpointProbeCandidate describes one read-only endpoint probe attempt.
type EndpointProbeCandidate struct {
	Label    string          `json:"label,omitempty"`
	Method   string          `json:"method,omitempty"`
	Endpoint string          `json:"endpoint"`
	Body     json.RawMessage `json:"body,omitempty"`
}

// EndpointProbeOptions controls probe behavior.
type EndpointProbeOptions struct {
	StopOnFirstSuccess bool `json:"stop_on_first_success"`
	IncludeHeaders     bool `json:"include_headers"`
	MaxBodyBytes       int  `json:"max_body_bytes"`
}

// EndpointProbeAttempt records one endpoint probe result.
type EndpointProbeAttempt struct {
	Label           string            `json:"label,omitempty"`
	Method          string            `json:"method"`
	Endpoint        string            `json:"endpoint"`
	EndpointFamily  string            `json:"endpoint_family,omitempty"`
	RequestURL      string            `json:"request_url,omitempty"`
	URL             string            `json:"url,omitempty"`
	AuthAttached    bool              `json:"auth_attached"`
	Success         bool              `json:"success"`
	UpstreamStatus  int               `json:"upstream_status,omitempty"`
	HTTPStatus      int               `json:"http_status,omitempty"`
	ContentType     string            `json:"content_type,omitempty"`
	JSONParseOK     bool              `json:"json_parse_ok"`
	JSONParseError  string            `json:"json_parse_error,omitempty"`
	FailureSource   string            `json:"failure_source,omitempty"`
	Classification  string            `json:"classification,omitempty"`
	Notes           []string          `json:"notes,omitempty"`
	DurationMS      int64             `json:"duration_ms"`
	RetryCount      int               `json:"retry_count,omitempty"`
	LocalError      string            `json:"local_error,omitempty"`
	UpstreamError   string            `json:"upstream_error,omitempty"`
	Error           string            `json:"error,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	BodyPreview     string            `json:"body_preview,omitempty"`
	ResponseBody    string            `json:"response_body,omitempty"`
	BodyTruncated   bool              `json:"body_truncated,omitempty"`
	MetaErrors      []string          `json:"meta_errors,omitempty"`
}

// EndpointProbeResult summarizes a batch of endpoint probes.
type EndpointProbeResult struct {
	ReadOnly           bool                   `json:"read_only"`
	StopOnFirstSuccess bool                   `json:"stop_on_first_success"`
	AttemptCount       int                    `json:"attempt_count"`
	SuccessCount       int                    `json:"success_count"`
	FirstSuccess       *EndpointProbeAttempt  `json:"first_success,omitempty"`
	Attempts           []EndpointProbeAttempt `json:"attempts"`
	AIGuidance         string                 `json:"ai_guidance,omitempty"`
}

// ProbeEndpoints tests one or more candidate GroupMe endpoints and returns the
// raw response details for each attempt. This is intentionally restricted to
// safe, read-only HTTP methods to avoid exposing a generic mutation primitive.
func (c *Client) ProbeEndpoints(ctx context.Context, candidates []EndpointProbeCandidate, opts EndpointProbeOptions) (*EndpointProbeResult, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("at least one endpoint candidate is required")
	}
	if len(candidates) > MaxProbeCandidates {
		return nil, fmt.Errorf("too many endpoint candidates: got %d, max %d", len(candidates), MaxProbeCandidates)
	}

	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = DefaultProbeMaxBodyBytes
	}

	result := &EndpointProbeResult{
		ReadOnly:           true,
		StopOnFirstSuccess: opts.StopOnFirstSuccess,
		Attempts:           make([]EndpointProbeAttempt, 0, len(candidates)),
		AIGuidance:         "Compare classification, failure_source, upstream_status, and notes across attempts. Subgroups/subtopics are discovered under /groups/{parent_group_id}/subgroups and read via /groups/{subgroup_id}/messages; do not model subgroup reads as /conversations/{id}/messages.",
	}

	for _, candidate := range candidates {
		attempt := c.probeEndpointCandidate(ctx, candidate, opts)
		result.Attempts = append(result.Attempts, attempt)
		if attempt.Success {
			result.SuccessCount++
			if result.FirstSuccess == nil {
				copy := attempt
				result.FirstSuccess = &copy
			}
			if opts.StopOnFirstSuccess {
				break
			}
		}
	}

	result.AttemptCount = len(result.Attempts)
	return result, nil
}

func (c *Client) probeEndpointCandidate(ctx context.Context, candidate EndpointProbeCandidate, opts EndpointProbeOptions) (attempt EndpointProbeAttempt) {
	start := time.Now()
	defer func() {
		if attempt.DurationMS == 0 {
			attempt.DurationMS = time.Since(start).Milliseconds()
		}
		attempt.Error = summarizeProbeError(attempt.LocalError, attempt.UpstreamError)
		if r := recover(); r != nil {
			attempt.Success = false
			attempt.LocalError = fmt.Sprintf("panic while probing endpoint: %v", r)
			attempt.Error = attempt.LocalError
			c.logger.Error("Probe panic recovered", "label", attempt.Label, "endpoint", attempt.Endpoint, "error", attempt.LocalError)
		}
	}()

	method := strings.ToUpper(strings.TrimSpace(candidate.Method))
	if method == "" {
		method = http.MethodGet
	}

	attempt = EndpointProbeAttempt{
		Label:    strings.TrimSpace(candidate.Label),
		Method:   method,
		Endpoint: strings.TrimSpace(candidate.Endpoint),
	}

	if attempt.Endpoint == "" {
		attempt.LocalError = "endpoint is required"
		return attempt
	}

	if !allowedProbeMethods[method] {
		attempt.LocalError = fmt.Sprintf("method %s is not allowed; only GET, HEAD, and OPTIONS are supported", method)
		return attempt
	}

	if len(strings.TrimSpace(string(candidate.Body))) > 0 {
		attempt.LocalError = "request body is not supported for read-only probes"
		return attempt
	}

	reqURL, err := c.resolveProbeURL(attempt.Endpoint)
	if err != nil {
		attempt.LocalError = err.Error()
		return attempt
	}
	attempt.RequestURL = reqURL
	attempt.URL = reqURL
	retries := 0

	for {
		req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
		if err != nil {
			attempt.LocalError = fmt.Sprintf("failed to create request: %v", err)
			c.logProbeLocalFailure(attempt)
			break
		}
		attempt.AuthAttached = c.attachAccessTokenHeader(req)
		c.logProbeRequest(attempt)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			attempt.LocalError = fmt.Sprintf("request failed: %v", err)
			c.logProbeLocalFailure(attempt)
			break
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			attempt.LocalError = fmt.Sprintf("failed to read response: %v", readErr)
			c.logProbeLocalFailure(attempt)
			break
		}

		if resp.StatusCode == http.StatusTooManyRequests && retries < MaxRetries {
			retries++
			waitTime := time.Duration(1<<uint(retries-1)) * time.Second
			c.logger.Warn("Probe rate limited", "url", reqURL, "wait_time", waitTime, "retry", retries)
			select {
			case <-ctx.Done():
				attempt.LocalError = ctx.Err().Error()
				attempt.DurationMS = time.Since(start).Milliseconds()
				attempt.RetryCount = retries
				c.logProbeLocalFailure(attempt)
				return attempt
			case <-time.After(waitTime):
				continue
			}
		}

		attempt.ContentType = strings.TrimSpace(resp.Header.Get("Content-Type"))
		attempt.UpstreamStatus = resp.StatusCode
		attempt.HTTPStatus = resp.StatusCode
		attempt.RetryCount = retries
		attempt.DurationMS = time.Since(start).Milliseconds()
		attempt.BodyPreview, attempt.BodyTruncated = previewProbeBody(c.redactProbePreview(respBody), opts.MaxBodyBytes)
		attempt.ResponseBody = attempt.BodyPreview
		attempt.MetaErrors = extractProbeMetaErrors(respBody)
		if opts.IncludeHeaders {
			attempt.ResponseHeaders = flattenHeaders(resp.Header)
		}

		jsonInspection := inspectProbeJSON(respBody)
		attempt.JSONParseOK = jsonInspection.ParseOK
		attempt.JSONParseError = jsonInspection.ParseError
		attempt.UpstreamError = describeProbeUpstreamError(resp.Status, resp.StatusCode, attempt.MetaErrors)
		attempt.Success = attempt.LocalError == "" && attempt.UpstreamError == ""
		classifyProbeAttempt(&attempt, jsonInspection)
		c.logProbeResponse(attempt)

		return attempt
	}

	classifyProbeAttempt(&attempt, probeJSONInspection{})
	attempt.RetryCount = retries
	return attempt
}

func (c *Client) resolveProbeURL(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("endpoint is required")
	}

	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return "", fmt.Errorf("invalid endpoint URL: %w", err)
		}
		if !allowedProbeHosts[strings.ToLower(parsed.Hostname())] {
			return "", fmt.Errorf("absolute URL host %q is not allowed; use a relative endpoint or an official GroupMe host", parsed.Hostname())
		}
		return parsed.String(), nil
	}

	return c.buildAPIURL(endpoint)
}

func extractProbeMetaErrors(body []byte) []string {
	if len(body) == 0 {
		return nil
	}

	var envelope struct {
		Meta struct {
			Errors []string `json:"errors"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	if len(envelope.Meta.Errors) == 0 {
		return nil
	}
	return envelope.Meta.Errors
}

func previewProbeBody(body []byte, maxBytes int) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	if maxBytes <= 0 || len(body) <= maxBytes {
		return string(bytes.ToValidUTF8(body, []byte("\uFFFD"))), false
	}
	return string(bytes.ToValidUTF8(body[:maxBytes], []byte("\uFFFD"))), true
}

func flattenHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	flat := make(map[string]string, len(headers))
	for key, values := range headers {
		flat[key] = strings.Join(values, ", ")
	}
	return flat
}

type probeJSONInspection struct {
	ParseOK    bool
	ParseError string
	Empty      bool
}

func inspectProbeJSON(body []byte) probeJSONInspection {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return probeJSONInspection{Empty: true}
	}

	var decoded interface{}
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return probeJSONInspection{
			ParseError: err.Error(),
		}
	}

	return probeJSONInspection{
		ParseOK: true,
		Empty:   isEffectivelyEmptyJSON(decoded),
	}
}

func isEffectivelyEmptyJSON(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case []interface{}:
		return len(typed) == 0
	case map[string]interface{}:
		if len(typed) == 0 {
			return true
		}
		if responseValue, ok := typed["response"]; ok {
			return isEffectivelyEmptyJSON(responseValue)
		}
		return false
	default:
		return false
	}
}

func describeProbeUpstreamError(statusLine string, statusCode int, metaErrors []string) string {
	parts := make([]string, 0, 2)
	if statusCode >= 400 {
		if strings.TrimSpace(statusLine) != "" {
			parts = append(parts, statusLine)
		} else {
			parts = append(parts, fmt.Sprintf("HTTP %d", statusCode))
		}
	}
	if len(metaErrors) > 0 {
		parts = append(parts, fmt.Sprintf("meta.errors: %s", strings.Join(metaErrors, "; ")))
	}
	return strings.Join(parts, " | ")
}

func summarizeProbeError(localError, upstreamError string) string {
	if localError != "" {
		return localError
	}
	return upstreamError
}

func (c *Client) redactProbePreview(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	return []byte(c.redactString(string(body)))
}

func classifyProbeAttempt(attempt *EndpointProbeAttempt, jsonInspection probeJSONInspection) {
	notes := make([]string, 0, 4)
	knowledge, knowledgeNotes := ProbeKnowledgeForEndpoint(attempt.Endpoint)
	if knowledge.Key != "" {
		attempt.EndpointFamily = knowledge.Key
		notes = append(notes, knowledgeNotes...)
		if knowledge.Notes != "" {
			notes = append(notes, knowledge.Notes)
		}
	}

	if !attempt.AuthAttached {
		notes = append(notes, "No X-Access-Token header was attached to the probe request.")
	}

	if attempt.LocalError != "" {
		attempt.FailureSource = "local"
		attempt.Classification = "inconclusive"
		notes = append(notes, "The request failed locally before an upstream response could be interpreted.")
		attempt.Notes = dedupeNotes(notes)
		return
	}

	if attempt.UpstreamError != "" {
		attempt.FailureSource = "upstream"
	}

	if attempt.ContentType != "" && strings.Contains(strings.ToLower(attempt.ContentType), "json") && attempt.JSONParseError != "" {
		notes = append(notes, "The server advertised JSON but the response body could not be parsed as JSON.")
	}
	if attempt.JSONParseError != "" && !strings.Contains(strings.ToLower(attempt.ContentType), "json") && attempt.ContentType != "" {
		notes = append(notes, "Non-JSON content type detected; body preview may be an HTML or plaintext error page.")
	}

	switch {
	case attempt.UpstreamStatus == http.StatusUnauthorized || attempt.UpstreamStatus == http.StatusForbidden:
		attempt.Classification = "likely auth issue"
	case attempt.UpstreamStatus == http.StatusNotFound:
		if knowledge.Key == "conversation_messages" {
			attempt.Classification = "likely unsupported resource"
		} else {
			attempt.Classification = "likely invalid path"
		}
	case attempt.UpstreamStatus == http.StatusBadRequest ||
		attempt.UpstreamStatus == http.StatusMethodNotAllowed ||
		attempt.UpstreamStatus == http.StatusNotAcceptable ||
		attempt.UpstreamStatus == http.StatusConflict ||
		attempt.UpstreamStatus == http.StatusUnprocessableEntity:
		attempt.Classification = "likely wrong parameter pattern"
	case attempt.UpstreamStatus >= 500:
		if knowledge.Key == "conversation_messages" {
			attempt.Classification = "likely unsupported resource"
		} else {
			attempt.Classification = "inconclusive"
		}
	case attempt.UpstreamStatus >= 200 && attempt.UpstreamStatus < 300 && attempt.UpstreamError == "":
		if jsonInspection.Empty || (attempt.Method == http.MethodHead && attempt.BodyPreview == "") {
			attempt.Classification = "likely valid but empty"
		} else {
			attempt.Classification = "likely valid undocumented endpoint"
		}
	default:
		if knowledge.Key == "conversation_messages" {
			attempt.Classification = "likely unsupported resource"
		} else if attempt.UpstreamError != "" {
			attempt.Classification = "inconclusive"
		}
	}

	if knowledge.Deprecated {
		notes = append(notes, "This endpoint family is deprecated for subgroup/subtopic message reads in this MCP server.")
	}
	if knowledge.Key == "subgroup_messages" {
		notes = append(notes, "Subgroup IDs currently behave like group IDs when reading messages.")
	}
	if attempt.Classification == "" {
		attempt.Classification = "inconclusive"
	}

	attempt.Notes = dedupeNotes(notes)
}

func dedupeNotes(notes []string) []string {
	if len(notes) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(notes))
	filtered := make([]string, 0, len(notes))
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if note == "" || seen[note] {
			continue
		}
		seen[note] = true
		filtered = append(filtered, note)
	}
	return filtered
}

func (c *Client) logProbeRequest(attempt EndpointProbeAttempt) {
	c.logger.Debug(
		"Probe request",
		"label", attempt.Label,
		"method", attempt.Method,
		"request_url", c.redactURLForLogs(attempt.RequestURL),
		"auth_attached", attempt.AuthAttached,
	)
}

func (c *Client) logProbeResponse(attempt EndpointProbeAttempt) {
	c.logger.Debug(
		"Probe response",
		"label", attempt.Label,
		"method", attempt.Method,
		"request_url", c.redactURLForLogs(attempt.RequestURL),
		"status", attempt.UpstreamStatus,
		"content_type", attempt.ContentType,
		"body_bytes", len(attempt.BodyPreview),
		"body_truncated", attempt.BodyTruncated,
		"retry_count", attempt.RetryCount,
		"success", attempt.Success,
		"local_error", attempt.LocalError,
		"upstream_error", attempt.UpstreamError,
	)
}

func (c *Client) logProbeLocalFailure(attempt EndpointProbeAttempt) {
	c.logger.Debug(
		"Probe local failure",
		"label", attempt.Label,
		"method", attempt.Method,
		"request_url", c.redactURLForLogs(attempt.RequestURL),
		"auth_attached", attempt.AuthAttached,
		"local_error", attempt.LocalError,
	)
}
