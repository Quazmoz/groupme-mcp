package client

import (
	"testing"
)

func TestCheckRateLimitUsesDocumentedEnvVars(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	t.Setenv("RATE_LIMIT_GLOBAL_RPM", "1")
	t.Setenv("RATE_LIMIT_BURST", "1")
	t.Setenv("RATE_LIMIT_REQUESTS", "")
	t.Setenv("RATE_LIMIT_WINDOW_SECONDS", "")

	globalRateLimiter.Lock()
	globalRateLimiter.requests = nil
	globalRateLimiter.Unlock()

	if err := checkRateLimit(); err != nil {
		t.Fatalf("first request should pass: %v", err)
	}
	if err := checkRateLimit(); err != nil {
		t.Fatalf("second request should use burst allowance: %v", err)
	}
	if err := checkRateLimit(); err == nil {
		t.Fatal("third request should exceed documented env based limit")
	}
}

func TestCheckRateLimitUsesLegacyEnvVars(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	t.Setenv("RATE_LIMIT_GLOBAL_RPM", "")
	t.Setenv("RATE_LIMIT_BURST", "")
	t.Setenv("RATE_LIMIT_REQUESTS", "1")
	t.Setenv("RATE_LIMIT_WINDOW_SECONDS", "60")

	globalRateLimiter.Lock()
	globalRateLimiter.requests = nil
	globalRateLimiter.Unlock()

	if err := checkRateLimit(); err != nil {
		t.Fatalf("first request should pass: %v", err)
	}
	if err := checkRateLimit(); err == nil {
		t.Fatal("second request should exceed legacy env based limit")
	}
}
