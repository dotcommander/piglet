package provider

import (
	"net/http"
	"strconv"
	"time"
)

// RetryAfterError wraps an API error that includes a Retry-After hint.
// Implements the retryAfterHint interface checked by core/stream.go.
type RetryAfterError struct {
	Err      error
	Duration time.Duration
}

func (e *RetryAfterError) Error() string             { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error             { return e.Err }
func (e *RetryAfterError) RetryAfter() time.Duration { return e.Duration }

// parseRetryAfter extracts a duration from the Retry-After header value.
// Supports integer seconds (the common case for API rate limits).
// Returns 0 if the header is empty or unparseable.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	// Try integer seconds first (most common for API providers)
	if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	// Try HTTP-date format
	if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
