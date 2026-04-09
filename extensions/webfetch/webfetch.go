// Package webfetch provides web fetch and search capabilities via multiple providers.
package webfetch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxBodyBytes  = 100 * 1024 // 100KB
	fetchTimeout  = 30 * time.Second
	searchTimeout = 15 * time.Second
	userAgent     = "piglet/1.0"

	cacheNSFetch   = "webfetch"
	cacheNSSearch  = "webfetch_search"
	cacheTTLFetch  = 24 * time.Hour
	cacheTTLSearch = time.Hour
)

const truncationNote = "\n\n[Content truncated at 100KB]"

// buildFetchResult formats a fetched page into the standard output format
// with optional title/URL header and truncation at maxBodyBytes.
func buildFetchResult(title, rawURL, body string) string {
	var sb strings.Builder
	if title != "" {
		fmt.Fprintf(&sb, "Title: %s\n\nURL Source: %s\n\n", title, rawURL)
	}
	sb.WriteString(body)
	content := sb.String()
	if len(content) > maxBodyBytes {
		content = content[:maxBodyBytes] + truncationNote
	}
	return content
}

// SearchResult holds a single search result.
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// HTTPError represents an HTTP error with status code and URL.
type HTTPError struct {
	URL        string
	StatusCode int
	Err        error
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("request %s: %v", e.URL, e.Err)
	}
	return fmt.Sprintf("request %s: HTTP %d", e.URL, e.StatusCode)
}

// Unwrap returns the underlying error.
func (e *HTTPError) Unwrap() error {
	return e.Err
}

// llmRefusalPrefixes detects LLM responses that are refusals rather than content.
// These should not be cached or returned as successful fetches.
var llmRefusalPrefixes = []string{
	"i am unable to access",
	"i cannot access",
	"i'm unable to access",
	"i can't access",
	"i do not have the ability to access",
	"i don't have access to",
	"as an ai",
	"i cannot browse",
	"i'm not able to access",
	"i cannot visit",
	"i cannot fetch",
	"i cannot open",
	"i cannot retrieve",
	"i'm not able to browse",
}

// isLLMRefusal returns true if the content looks like an LLM refusal rather
// than actual page content. This prevents caching garbage responses.
func isLLMRefusal(content string) bool {
	// Only lowercase the first 100 bytes — all refusal prefixes are short.
	s := strings.TrimSpace(content)
	if len(s) > 100 {
		s = s[:100]
	}
	lower := strings.ToLower(s)
	for _, prefix := range llmRefusalPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// isRecoverable returns true if the error might succeed with a different provider.
func isRecoverable(err error) bool {
	if err == nil {
		return false
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		// Network errors (status 0) are recoverable
		if httpErr.StatusCode == 0 {
			return true
		}
		// 2xx "soft" failures (e.g. 204 = page needs JS) are always recoverable.
		if httpErr.StatusCode >= 200 && httpErr.StatusCode < 300 {
			return true
		}
		// 4xx client errors (except 401, 403, 429, 451) are not recoverable.
		// 451 (Unavailable For Legal Reasons) is often a proxy/reader issue, not the target URL.
		if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 &&
			httpErr.StatusCode != 401 && httpErr.StatusCode != 403 &&
			httpErr.StatusCode != 429 && httpErr.StatusCode != 451 {
			return false
		}
		// 5xx server errors and 429 rate limits are recoverable
		return true
	}

	// Other errors (parsing, etc.) might be recoverable
	return true
}

// FetchProvider defines the interface for content fetching.
type FetchProvider interface {
	Name() string
	Fetch(ctx context.Context, rawURL string) (string, error)
}

// SearchProvider defines the interface for web search.
type SearchProvider interface {
	Name() string
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
}
