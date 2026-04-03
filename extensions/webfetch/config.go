package webfetch

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"gopkg.in/yaml.v3"
)

// Config holds configuration for webfetch providers.
type Config struct {
	JinaAPIKey       string           `yaml:"jina_api_key"`
	PerplexityAPIKey string           `yaml:"perplexity_api_key"`
	GeminiAPIKey     string           `yaml:"gemini_api_key"`
	BraveAPIKey      string           `yaml:"brave_api_key"`
	ExaAPIKey        string           `yaml:"exa_api_key"`
	GitHub           GitHubConfig     `yaml:"github"`
	Gemini           GeminiConfig     `yaml:"gemini_config"`
	Perplexity       PerplexityConfig `yaml:"perplexity_config"`
	Exa              ExaConfig        `yaml:"exa_config"`
	Jina             JinaConfig       `yaml:"jina_config"`
	Brave            BraveConfig      `yaml:"brave_config"`
	DuckDuckGo       DuckDuckGoConfig `yaml:"duckduckgo_config"`
}

// GeminiConfig holds Gemini API endpoint settings.
type GeminiConfig struct {
	APIBase string `yaml:"api_base"`
	Model   string `yaml:"model"`
}

// PerplexityConfig holds Perplexity API endpoint settings.
type PerplexityConfig struct {
	APIURL string `yaml:"api_url"`
	Model  string `yaml:"model"`
}

// ExaConfig holds Exa API endpoint settings.
type ExaConfig struct {
	SearchURL   string `yaml:"search_url"`
	ContentsURL string `yaml:"contents_url"`
}

// JinaConfig holds Jina API endpoint settings.
type JinaConfig struct {
	ReaderBase string `yaml:"reader_base"`
	SearchBase string `yaml:"search_base"`
}

// BraveConfig holds Brave API endpoint settings.
type BraveConfig struct {
	SearchURL string `yaml:"search_url"`
}

// DuckDuckGoConfig holds DuckDuckGo search endpoint settings.
type DuckDuckGoConfig struct {
	SearchURL string `yaml:"search_url"`
}

// DefaultGeminiConfig returns the default Gemini configuration.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		APIBase: "https://generativelanguage.googleapis.com/v1beta",
		Model:   "gemini-2.0-flash",
	}
}

// DefaultPerplexityConfig returns the default Perplexity configuration.
func DefaultPerplexityConfig() PerplexityConfig {
	return PerplexityConfig{
		APIURL: "https://api.perplexity.ai/chat/completions",
		Model:  "llama-3.1-sonar-small-128k-online",
	}
}

// DefaultExaConfig returns the default Exa configuration.
func DefaultExaConfig() ExaConfig {
	return ExaConfig{
		SearchURL:   "https://api.exa.ai/search",
		ContentsURL: "https://api.exa.ai/contents",
	}
}

// DefaultJinaConfig returns the default Jina configuration.
func DefaultJinaConfig() JinaConfig {
	return JinaConfig{
		ReaderBase: "https://r.jina.ai/",
		SearchBase: "https://s.jina.ai/",
	}
}

// DefaultBraveConfig returns the default Brave configuration.
func DefaultBraveConfig() BraveConfig {
	return BraveConfig{
		SearchURL: "https://api.search.brave.com/res/v1/web/search",
	}
}

// DefaultDuckDuckGoConfig returns the default DuckDuckGo configuration.
func DefaultDuckDuckGoConfig() DuckDuckGoConfig {
	return DuckDuckGoConfig{
		SearchURL: "https://html.duckduckgo.com/html/",
	}
}

// LoadConfig reads configuration from the namespaced extension directory
// (~/.config/piglet/extensions/webfetch/webfetch.yaml), falling back to the
// flat location (~/.config/piglet/webfetch.yaml) for backward compatibility.
// If neither exists, it creates one with default values in the namespaced directory.
func LoadConfig() (*Config, error) {
	extDir, err := xdg.ExtensionDir("webfetch")
	if err != nil {
		return nil, fmt.Errorf("get extension dir: %w", err)
	}
	extPath := filepath.Join(extDir, "webfetch.yaml")

	data, err := os.ReadFile(extPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		// Fallback: try flat location
		configDir, dirErr := xdg.ConfigDir()
		if dirErr != nil {
			return nil, fmt.Errorf("get config dir: %w", dirErr)
		}
		flatPath := filepath.Join(configDir, "webfetch.yaml")
		data, err = os.ReadFile(flatPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Neither exists — create default in namespaced dir
				return createDefaultConfig(extPath)
			}
			return nil, fmt.Errorf("read config: %w", err)
		}
		// Migrate from flat to namespaced
		if err := os.MkdirAll(extDir, 0o755); err == nil {
			_ = xdg.WriteFileAtomic(extPath, data)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// GitHub defaults are set in GitHubConfig struct tags / NewGitHubClient.
	// Don't override explicit false values from config.

	cfg.applyDefaults()
	return &cfg, nil
}

// applyDefaults fills zero-value provider config fields with their defaults.
func (c *Config) applyDefaults() {
	if c.Gemini.APIBase == "" && c.Gemini.Model == "" {
		c.Gemini = DefaultGeminiConfig()
	}
	if c.Perplexity.APIURL == "" && c.Perplexity.Model == "" {
		c.Perplexity = DefaultPerplexityConfig()
	}
	if c.Exa.SearchURL == "" && c.Exa.ContentsURL == "" {
		c.Exa = DefaultExaConfig()
	}
	if c.Jina.ReaderBase == "" && c.Jina.SearchBase == "" {
		c.Jina = DefaultJinaConfig()
	}
	if c.Brave.SearchURL == "" {
		c.Brave = DefaultBraveConfig()
	}
	if c.DuckDuckGo.SearchURL == "" {
		c.DuckDuckGo = DefaultDuckDuckGoConfig()
	}
}

func createDefaultConfig(path string) (*Config, error) {
	defaultConfig := &Config{
		GitHub: GitHubConfig{
			Enabled:        true,
			SkipLargeRepos: true,
		},
		Gemini:     DefaultGeminiConfig(),
		Perplexity: DefaultPerplexityConfig(),
		Exa:        DefaultExaConfig(),
		Jina:       DefaultJinaConfig(),
		Brave:      DefaultBraveConfig(),
		DuckDuckGo: DefaultDuckDuckGoConfig(),
	}
	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal default config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create extension dir: %w", err)
	}
	if err := xdg.WriteFileAtomic(path, data); err != nil {
		return nil, fmt.Errorf("create default config: %w", err)
	}
	return defaultConfig, nil
}

// rateLimiter implements a simple rate limiter for API calls.
type rateLimiter struct {
	interval time.Duration
	mu       sync.Mutex
	lastCall time.Time
}

// newRateLimiter creates a rate limiter with the given minimum interval between calls.
func newRateLimiter(interval time.Duration) *rateLimiter {
	return &rateLimiter{interval: interval}
}

// Wait blocks until the minimum interval has passed since the last call.
func (r *rateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsed := time.Since(r.lastCall)
	if elapsed < r.interval {
		wait := r.interval - elapsed
		slog.Debug("rate limiter waiting", "wait", wait)
		time.Sleep(wait)
	}
	r.lastCall = time.Now()
}

// PerplexityRateLimitInterval is the minimum time between Perplexity API calls (10 req/min).
const PerplexityRateLimitInterval = 6 * time.Second
