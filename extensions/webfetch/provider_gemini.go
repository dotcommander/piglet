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

//go:embed defaults/gemini-fetch-prompt.txt
var rawGeminiFetchPrompt string

//go:embed defaults/gemini-search-prompt.txt
var rawGeminiSearchPrompt string

var (
	geminiPromptOnce         sync.Once
	activeGeminiFetchPrompt  string
	activeGeminiSearchPrompt string
)

func loadGeminiPrompts() {
	geminiPromptOnce.Do(func() {
		activeGeminiFetchPrompt = xdg.LoadOrCreateExt("webfetch", "gemini-fetch-prompt.txt", strings.TrimSpace(rawGeminiFetchPrompt))
		activeGeminiSearchPrompt = xdg.LoadOrCreateExt("webfetch", "gemini-search-prompt.txt", strings.TrimSpace(rawGeminiSearchPrompt))
	})
}

// GeminiProvider implements FetchProvider and SearchProvider using Google Gemini API.
type GeminiProvider struct {
	apiBase string
	model   string
	apiKey  string
	http    *http.Client
}

// NewGeminiProvider creates a GeminiProvider with the given API key and endpoint config.
// Returns nil if apiKey is empty.
func NewGeminiProvider(apiKey string, cfg GeminiConfig) *GeminiProvider {
	if apiKey == "" {
		return nil
	}

	return &GeminiProvider{
		apiBase: cfg.APIBase,
		model:   cfg.Model,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the provider name for logging.
func (g *GeminiProvider) Name() string {
	return "gemini"
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
	Error      *geminiError      `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Fetch retrieves content by asking Gemini to summarize the URL.
func (g *GeminiProvider) Fetch(ctx context.Context, rawURL string) (string, error) {
	loadGeminiPrompts()
	apiURL := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.apiBase, g.model, g.apiKey)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{
						Text: fmt.Sprintf(activeGeminiFetchPrompt, rawURL),
					},
				},
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return "", &HTTPError{URL: apiURL, StatusCode: 0, Err: err}
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", &HTTPError{URL: apiURL, StatusCode: resp.StatusCode}
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respData, &geminiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if geminiResp.Error != nil {
		return "", fmt.Errorf("gemini error [%d]: %s", geminiResp.Error.Code, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from gemini")
	}

	content := geminiResp.Candidates[0].Content.Parts[0].Text

	// Detect refusal responses — Gemini sometimes returns "I cannot access URLs"
	// as a valid 200 response. Treat these as errors so the chain falls through.
	if isLLMRefusal(content) {
		return "", &HTTPError{
			URL:        rawURL,
			StatusCode: 204,
			Err:        fmt.Errorf("gemini refused to fetch: content appears to be a refusal"),
		}
	}

	if len(content) > maxBodyBytes {
		content = content[:maxBodyBytes] + "\n\n[Content truncated at 100KB]"
	}

	slog.Debug("gemini fetch completed", "url", rawURL)
	return content, nil
}

// Search queries Gemini for search results.
func (g *GeminiProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	loadGeminiPrompts()

	if limit <= 0 {
		limit = 5
	}

	apiURL := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.apiBase, g.model, g.apiKey)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{
						Text: fmt.Sprintf(activeGeminiSearchPrompt, query, limit),
					},
				},
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, &HTTPError{URL: apiURL, StatusCode: 0, Err: err}
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &HTTPError{URL: apiURL, StatusCode: resp.StatusCode}
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respData, &geminiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("gemini error [%d]: %s", geminiResp.Error.Code, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from gemini")
	}

	// Gemini returns text, not structured results. Parse it into SearchResult format.
	content := geminiResp.Candidates[0].Content.Parts[0].Text
	results := parseGeminiSearchResults(content, limit)

	slog.Debug("gemini search completed", "query", query, "results", len(results))
	return results, nil
}

// parseGeminiSearchResults extracts structured results from Gemini text response.
func parseGeminiSearchResults(content string, limit int) []SearchResult {
	return parseLLMSearchResults("Gemini Search Results", "https://gemini.google.com", content, limit)
}
