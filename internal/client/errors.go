package client

import (
	"errors"
	"fmt"
)

// Common errors that can be checked with errors.Is()
var (
	ErrNotFound       = errors.New("resource not found")
	ErrUnauthorized   = errors.New("unauthorized - invalid or expired token")
	ErrRateLimited    = errors.New("rate limited - too many requests")
	ErrServerError    = errors.New("GroupMe server error")
	ErrInvalidRequest = errors.New("invalid request")
)

// APIError represents an error returned by the GroupMe API.
type APIError struct {
	StatusCode int
	Endpoint   string
	Message    string
	Err        error // Underlying error for errors.Is/As
}

func (e *APIError) Error() string {
	if e.Endpoint != "" {
		return fmt.Sprintf("GroupMe API error (status %d) on %s: %s", e.StatusCode, e.Endpoint, e.Message)
	}
	return fmt.Sprintf("GroupMe API error (status %d): %s", e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// NewAPIError creates a new APIError with the appropriate underlying error type.
func NewAPIError(statusCode int, endpoint, message string) *APIError {
	var err error
	switch {
	case statusCode == 401 || statusCode == 403:
		err = ErrUnauthorized
	case statusCode == 404:
		err = ErrNotFound
	case statusCode == 429:
		err = ErrRateLimited
	case statusCode >= 500:
		err = ErrServerError
	case statusCode >= 400:
		err = ErrInvalidRequest
	}
	return &APIError{
		StatusCode: statusCode,
		Endpoint:   endpoint,
		Message:    message,
		Err:        err,
	}
}

// IsNotFound returns true if the error indicates a resource was not found.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsRateLimited returns true if the error indicates rate limiting.
func IsRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

// IsUnauthorized returns true if the error indicates an auth problem.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}
