package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/dotcommander/piglet/extensions/cache"
)

// Client performs web fetch and search operations with fallback support.
// The zero value is not usable; use New, NewWithConfig, Default, or NewForTest.
type Client struct {
	fetchProviders  []FetchProvider
	searchProviders []SearchProvider
	http            *http.Client
	github          *GitHubClient
	storage         *Storage
	ddgSearchURL    string
}

// New creates a Client with custom provider lists (used for testing).
func New(fetchProviders []FetchProvider, searchProviders []SearchProvider, github *GitHubClient) *Client {
	return &Client{
		fetchProviders:  fetchProviders,
		searchProviders: searchProviders,
		http: &http.Client{
			Timeout: fetchTimeout,
		},
		github:       github,
		storage:      NewStorage(),
		ddgSearchURL: DefaultDuckDuckGoConfig().SearchURL,
	}
}

// NewWithConfig creates a Client with providers based on the given config.
// Fetch priority: Colly → Jina → Rod → agent-browser → Gemini → Perplexity.
// Search priority: Brave → Exa → Gemini → Perplexity → Jina → DuckDuckGo.
func NewWithConfig(cfg *Config) *Client {
	var fetchProviders []FetchProvider
	var searchProviders []SearchProvider

	// --- Resolve all API keys (config then env) ---

	exaKey := cfg.ExaAPIKey
	if exaKey == "" {
		exaKey = os.Getenv("EXA_API_KEY")
	}
	braveKey := cfg.BraveAPIKey
	if braveKey == "" {
		braveKey = os.Getenv("BRAVE_API_KEY")
	}
	geminiKey := cfg.GeminiAPIKey
	if geminiKey == "" {
		geminiKey = os.Getenv("GEMINI_API_KEY")
	}
	perplexityKey := cfg.PerplexityAPIKey
	if perplexityKey == "" {
		perplexityKey = os.Getenv("PERPLEXITY_API_KEY")
	}
	jinaKey := cfg.JinaAPIKey
	if jinaKey == "" {
		jinaKey = os.Getenv("JINA_API_KEY")
	}

	// --- Create shared provider instances ---

	jina := NewJinaProvider(jinaKey, cfg.Jina)
	gemini := NewGeminiProvider(geminiKey, cfg.Gemini)
	perplexity := NewPerplexityProvider(perplexityKey, cfg.Perplexity)

	// --- Fetch providers: local fetchers → reader proxy → headless browser → LLM ---
	// Exa and Brave are search-only; they don't belong in the fetch chain.

	fetchProviders = append(fetchProviders, NewCollyProvider())
	fetchProviders = append(fetchProviders, jina)
	fetchProviders = append(fetchProviders, NewRodProvider())
	if ab := NewAgentBrowserProvider(); ab != nil {
		fetchProviders = append(fetchProviders, ab)
	}
	if gemini != nil {
		fetchProviders = append(fetchProviders, gemini)
	}
	if perplexity != nil {
		fetchProviders = append(fetchProviders, perplexity)
	}

	// --- Search providers: structured APIs first, then LLM, then Jina ---

	if brave := NewBraveProvider(braveKey, cfg.Brave); brave != nil {
		searchProviders = append(searchProviders, brave)
	}
	if exa := NewExaProvider(exaKey, cfg.Exa); exa != nil {
		searchProviders = append(searchProviders, exa)
	}
	if gemini != nil {
		searchProviders = append(searchProviders, gemini)
	}
	if perplexity != nil {
		searchProviders = append(searchProviders, perplexity)
	}
	searchProviders = append(searchProviders, jina)

	// Create GitHub client if enabled
	var github *GitHubClient
	if cfg.GitHub.Enabled {
		github = NewGitHubClient(&cfg.GitHub)
	}

	c := New(fetchProviders, searchProviders, github)
	c.ddgSearchURL = cfg.DuckDuckGo.SearchURL
	return c
}

// Default returns a Client with only Jina provider (no API keys required).
func Default() *Client {
	cfg := &Config{
		GitHub: GitHubConfig{
			Enabled:        true,
			SkipLargeRepos: true,
		},
	}
	cfg.applyDefaults()
	return NewWithConfig(cfg)
}

// NewForTest creates a Client with mock Jina providers for testing.
func NewForTest(readerBase, searchBase string) *Client {
	jina := NewJinaProviderWithBase(readerBase, searchBase, "")
	return New([]FetchProvider{jina}, []SearchProvider{jina}, nil)
}

// Fetch retrieves content from the given URL with provider fallback.
// If raw is false, content is fetched via reader providers (returns clean markdown).
// If raw is true, the URL is fetched directly.
func (c *Client) Fetch(ctx context.Context, rawURL string, raw bool) (string, error) {
	if cached, ok := cache.Get(cacheNSFetch, rawURL); ok {
		return cached, nil
	}

	// Raw mode fetches directly without provider fallback
	if raw {
		content, err := c.fetchRaw(ctx, rawURL)
		if err != nil {
			return "", err
		}
		// Store full content before returning
		c.storage.StoreFetch(rawURL, content)
		if err := cache.Set(cacheNSFetch, rawURL, content, cacheTTLFetch); err != nil {
			slog.Debug("cache write failed", "url", rawURL, "error", err)
		}
		return content, nil
	}

	// Try GitHub first if enabled
	if c.github != nil {
		result, err := c.github.Fetch(ctx, rawURL)
		if err != nil {
			return "", err
		}
		if result != nil {
			content := FormatGitHubResult(result)
			c.storage.StoreFetch(rawURL, content)
			if err := cache.Set(cacheNSFetch, rawURL, content, cacheTTLFetch); err != nil {
				slog.Debug("cache write failed", "url", rawURL, "error", err)
			}
			return content, nil
		}
	}

	// Try each provider in order
	var lastErr error
	for _, provider := range c.fetchProviders {
		content, err := provider.Fetch(ctx, rawURL)
		if err == nil {
			// Store full content before returning
			c.storage.StoreFetch(rawURL, content)
			if err := cache.Set(cacheNSFetch, rawURL, content, cacheTTLFetch); err != nil {
				slog.Debug("cache write failed", "url", rawURL, "error", err)
			}
			return content, nil
		}

		slog.Debug("fetch provider failed",
			"provider", provider.Name(),
			"url", rawURL,
			"error", err)

		lastErr = err

		// If error is not recoverable, don't try next provider
		if !isRecoverable(err) {
			break
		}
	}

	return "", fmt.Errorf("all fetch providers failed: %w", lastErr)
}

// fetchRaw fetches the URL directly without any provider.
func (c *Client) fetchRaw(ctx context.Context, rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	return httpGet(ctx, c.http, rawURL, nil)
}

// Search queries providers and returns up to limit results with fallback.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	cacheKey := fmt.Sprintf("%s:%d", query, limit)
	if cached, ok := cache.Get(cacheNSSearch, cacheKey); ok {
		var results []SearchResult
		if err := json.Unmarshal([]byte(cached), &results); err == nil {
			return results, nil
		}
	}

	// Try each provider in order
	var lastErr error
	for _, provider := range c.searchProviders {
		results, err := provider.Search(ctx, query, limit)
		if err == nil {
			// Store results before returning
			c.storage.StoreSearch(query, results)
			if data, err := json.Marshal(results); err == nil {
				if err := cache.Set(cacheNSSearch, cacheKey, string(data), cacheTTLSearch); err != nil {
					slog.Debug("cache write failed", "query", query, "error", err)
				}
			}
			return results, nil
		}

		slog.Debug("search provider failed",
			"provider", provider.Name(),
			"query", query,
			"error", err)

		lastErr = err

		// If error is not recoverable, don't try next provider
		if !isRecoverable(err) {
			break
		}
	}

	// Fallback: fetch a DuckDuckGo search page via reader providers
	slog.Debug("search providers exhausted, falling back to web fetch", "query", query)
	ddgURL := c.ddgSearchURL + "?q=" + url.QueryEscape(query)
	content, err := c.Fetch(ctx, ddgURL, false)
	if err != nil {
		return nil, fmt.Errorf("all search providers failed (fetch fallback also failed): %w", lastErr)
	}

	return []SearchResult{{
		Title:       "Web search results for: " + query,
		URL:         ddgURL,
		Description: content,
	}}, nil
}

// FormatResults renders a slice of SearchResults as a markdown list.
func FormatResults(results []SearchResult) string {
	if len(results) == 0 {
		return "No results found."
	}
	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	return strings.TrimRight(b.String(), "\n")
}

// GetStorage returns the session storage for cached results.
func (c *Client) GetStorage() *Storage {
	return c.storage
}
