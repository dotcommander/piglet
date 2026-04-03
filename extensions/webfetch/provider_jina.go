package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// JinaProvider implements FetchProvider and SearchProvider using Jina AI readers.
type JinaProvider struct {
	readerBase string
	searchBase string
	apiKey     string
	http       *http.Client
}

// NewJinaProvider creates a JinaProvider with the given API key and endpoint config.
func NewJinaProvider(apiKey string, cfg JinaConfig) *JinaProvider {
	return NewJinaProviderWithBase(cfg.ReaderBase, cfg.SearchBase, apiKey)
}

// NewJinaProviderWithBase creates a JinaProvider with custom base URLs (for testing).
func NewJinaProviderWithBase(readerBase, searchBase string, apiKey string) *JinaProvider {
	return &JinaProvider{
		readerBase: readerBase,
		searchBase: searchBase,
		apiKey:     apiKey,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the provider name for logging.
func (j *JinaProvider) Name() string {
	return "jina"
}

// Fetch retrieves content from the given URL via the Jina reader.
func (j *JinaProvider) Fetch(ctx context.Context, rawURL string) (string, error) {
	fetchURL := j.readerBase + rawURL

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if j.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+j.apiKey)
	}

	resp, err := j.http.Do(req)
	if err != nil {
		return "", &HTTPError{URL: fetchURL, StatusCode: 0, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", &HTTPError{URL: fetchURL, StatusCode: resp.StatusCode}
	}

	limited := io.LimitReader(resp.Body, maxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	content := string(data)
	if len(data) > maxBodyBytes {
		content = content[:maxBodyBytes] + "\n\n[Content truncated at 100KB]"
	}

	return content, nil
}

// Search queries the Jina search endpoint.
func (j *JinaProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	searchURL := j.searchBase + url.PathEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if j.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+j.apiKey)
	}

	resp, err := j.http.Do(req)
	if err != nil {
		return nil, &HTTPError{URL: searchURL, StatusCode: 0, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, &HTTPError{URL: searchURL, StatusCode: resp.StatusCode}
	}

	var envelope jinaSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	items := envelope.Data
	if len(items) > limit {
		items = items[:limit]
	}

	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		desc := item.Description
		if desc == "" {
			desc = strings.TrimSpace(item.Content)
			if len(desc) > 200 {
				desc = desc[:200] + "…"
			}
		}
		results = append(results, SearchResult{
			Title:       item.Title,
			URL:         item.URL,
			Description: desc,
		})
	}

	slog.Debug("jina search completed", "query", query, "results", len(results))
	return results, nil
}

// jinaSearchResponse is the JSON envelope returned by s.jina.ai.
type jinaSearchResponse struct {
	Data []jinaSearchItem `json:"data"`
}

type jinaSearchItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Content     string `json:"content"`
}
