package modelsdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/sdk"
)

const (
	DefaultAPIURL = "https://models.dev/api.json"
	cacheFile     = ".models-cache.json"
	cacheMaxAge   = 24 * time.Hour
)

// modelsdevConfig holds configurable settings for models.dev sync.
type modelsdevConfig struct {
	APIURL string `yaml:"api_url"`
}

func loadModelsdevConfig() string {
	cfg := xdg.LoadYAMLExt("modelsdev", "modelsdev.yaml", modelsdevConfig{APIURL: DefaultAPIURL})
	if cfg.APIURL == "" {
		return DefaultAPIURL
	}
	return cfg.APIURL
}

type apiResponse map[string]providerData

type providerData struct {
	ID     string               `json:"id"`
	Models map[string]modelData `json:"models"`
}

type modelData struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Limit modelLimit `json:"limit"`
}

type modelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// cache is the on-disk format for the API response cache.
type cache struct {
	FetchedAt time.Time   `json:"fetched_at"`
	Data      apiResponse `json:"data"`
}

// CacheStale returns true if the cache is missing or older than 24h.
func CacheStale() bool {
	c := readCache()
	if c == nil {
		return true
	}
	return time.Since(c.FetchedAt) > cacheMaxAge
}

// FetchOverrides fetches the models.dev API and returns a map of overrides
// keyed by "provider/id" (lowercased). The host uses these to regenerate
// models.yaml from its embedded curated list, preserving cost and metadata.
func FetchOverrides(ctx context.Context) (map[string]sdk.ModelOverride, error) {
	return fetchOverridesFromURL(ctx, loadModelsdevConfig())
}

func fetchOverridesFromURL(ctx context.Context, url string) (map[string]sdk.ModelOverride, error) {
	data, err := fetch(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch models.dev: %w", err)
	}

	_ = writeCache(&cache{FetchedAt: time.Now(), Data: data})

	apiIndex := indexAPI(data)
	overrides := make(map[string]sdk.ModelOverride, len(apiIndex))
	for key, md := range apiIndex {
		overrides[key] = sdk.ModelOverride{
			Name:          md.Name,
			ContextWindow: md.Limit.Context,
			MaxTokens:     md.Limit.Output,
		}
	}
	return overrides, nil
}

// Refresh fetches API data and writes models.yaml via the host RPC,
// preserving cost data and metadata from the embedded curated list.
func Refresh(ctx context.Context, ext *sdk.Extension) (int, error) {
	overrides, err := FetchOverrides(ctx)
	if err != nil {
		return 0, err
	}
	return ext.WriteModels(ctx, overrides)
}

func fetch(ctx context.Context, url string) (apiResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "piglet")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	return result, nil
}

// canonicalProvider maps models.dev provider keys to the canonical names
// used in piglet's curated model list. The API may list the same models
// under multiple provider keys (e.g. "zai-coding-plan", "zhipuai-coding-plan"
// both map to our "zai" provider).
var canonicalProvider = map[string]string{
	"zai":                 "zai",
	"zai-coding-plan":     "zai",
	"zhipuai-coding-plan": "zai",
	"anthropic":           "anthropic",
	"openai":              "openai",
	"google":              "google",
	"xai":                 "xai",
	"groq":                "groq",
}

func indexAPI(data apiResponse) map[string]modelData {
	index := make(map[string]modelData)
	for provName, prov := range data {
		canonical, ok := canonicalProvider[provName]
		if !ok {
			continue // skip providers we don't track
		}
		for _, md := range prov.Models {
			key := canonical + "/" + strings.ToLower(md.ID)
			// First match wins — prefer the primary provider key
			if _, exists := index[key]; !exists {
				index[key] = md
			}
		}
	}
	return index
}

func cachePath() (string, error) {
	dir, err := xdg.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheFile), nil
}

func readCache() *cache {
	path, err := cachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return &c
}

func writeCache(c *cache) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	return xdg.WriteFileAtomic(path, data)
}
