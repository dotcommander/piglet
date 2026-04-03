package webfetch

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed defaults/fetch-prompt.txt
var rawFetchPrompt string

//go:embed defaults/search-prompt.txt
var rawSearchPrompt string

var (
	perplexityPromptOnce sync.Once
	activeFetchPrompt    string
	activeSearchPrompt   string
)

func loadPerplexityPrompts() {
	perplexityPromptOnce.Do(func() {
		activeFetchPrompt = xdg.LoadOrCreateExt("webfetch", "fetch-prompt.txt", strings.TrimSpace(rawFetchPrompt))
		activeSearchPrompt = xdg.LoadOrCreateExt("webfetch", "search-prompt.txt", strings.TrimSpace(rawSearchPrompt))
	})
}

// PerplexityProvider implements FetchProvider and SearchProvider using Perplexity API.
type PerplexityProvider struct {
	apiURL      string
	model       string
	apiKey      string
	http        *http.Client
	rateLimiter *rateLimiter
}

// NewPerplexityProvider creates a PerplexityProvider with the given API key and endpoint config.
// Returns nil if apiKey is empty.
func NewPerplexityProvider(apiKey string, cfg PerplexityConfig) *PerplexityProvider {
	if apiKey == "" {
		return nil
	}

	return &PerplexityProvider{
		apiURL: cfg.APIURL,
		model:  cfg.Model,
		apiKey: apiKey,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimiter: newRateLimiter(PerplexityRateLimitInterval),
	}
}

// Name returns the provider name for logging.
func (p *PerplexityProvider) Name() string {
	return "perplexity"
}

type perplexityRequest struct {
	Model    string              `json:"model"`
	Messages []perplexityMessage `json:"messages"`
}

type perplexityMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type perplexityResponse struct {
	Choices []perplexityChoice `json:"choices"`
	Error   *perplexityError   `json:"error,omitempty"`
}

type perplexityChoice struct {
	Message perplexityMessage `json:"message"`
}

type perplexityError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Fetch retrieves content by asking Perplexity to summarize the URL.
func (p *PerplexityProvider) Fetch(ctx context.Context, rawURL string) (string, error) {
	loadPerplexityPrompts()
	p.rateLimiter.Wait()

	reqBody := perplexityRequest{
		Model: p.model,
		Messages: []perplexityMessage{
			{
				Role:    "user",
				Content: fmt.Sprintf(activeFetchPrompt, rawURL),
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return "", &HTTPError{URL: p.apiURL, StatusCode: 0, Err: err}
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", &HTTPError{URL: p.apiURL, StatusCode: resp.StatusCode}
	}

	var perplexResp perplexityResponse
	if err := json.Unmarshal(respData, &perplexResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if perplexResp.Error != nil {
		return "", fmt.Errorf("perplexity error: %s", perplexResp.Error.Message)
	}

	if len(perplexResp.Choices) == 0 {
		return "", fmt.Errorf("no response from perplexity")
	}

	content := perplexResp.Choices[0].Message.Content

	// Detect refusal responses — LLMs sometimes return "I cannot access URLs"
	// as a valid response. Treat these as errors so the chain falls through.
	if isLLMRefusal(content) {
		return "", &HTTPError{
			URL:        rawURL,
			StatusCode: 204,
			Err:        fmt.Errorf("perplexity refused to fetch: content appears to be a refusal"),
		}
	}

	if len(content) > maxBodyBytes {
		content = content[:maxBodyBytes] + "\n\n[Content truncated at 100KB]"
	}

	slog.Debug("perplexity fetch completed", "url", rawURL)
	return content, nil
}

// Search queries Perplexity for search results.
func (p *PerplexityProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	loadPerplexityPrompts()

	if limit <= 0 {
		limit = 5
	}

	p.rateLimiter.Wait()

	reqBody := perplexityRequest{
		Model: p.model,
		Messages: []perplexityMessage{
			{
				Role:    "user",
				Content: fmt.Sprintf(activeSearchPrompt, query, limit),
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, &HTTPError{URL: p.apiURL, StatusCode: 0, Err: err}
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &HTTPError{URL: p.apiURL, StatusCode: resp.StatusCode}
	}

	var perplexResp perplexityResponse
	if err := json.Unmarshal(respData, &perplexResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if perplexResp.Error != nil {
		return nil, fmt.Errorf("perplexity error: %s", perplexResp.Error.Message)
	}

	if len(perplexResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from perplexity")
	}

	// Perplexity returns text, not structured results. Parse it into SearchResult format.
	content := perplexResp.Choices[0].Message.Content
	results := parsePerplexitySearchResults(content, limit)

	slog.Debug("perplexity search completed", "query", query, "results", len(results))
	return results, nil
}

// parsePerplexitySearchResults extracts structured results from Perplexity text response.
func parsePerplexitySearchResults(content string, limit int) []SearchResult {
	return parseLLMSearchResults("Perplexity Search Results", "https://perplexity.ai", content, limit)
}

// parseLLMSearchResults wraps unstructured LLM text as a single SearchResult.
func parseLLMSearchResults(title, url, content string, limit int) []SearchResult {
	if limit <= 0 {
		limit = 1
	}
	results := []SearchResult{
		{
			Title:       title,
			URL:         url,
			Description: truncateString(content, 500),
		},
	}
	return results[:min(len(results), limit)]
}

func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}
