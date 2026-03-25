package modelsdev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/provider"
)

const (
	cacheFile = ".models-cache.json"
	cacheMaxAge = 24 * time.Hour
)

// cache is the on-disk format for the models.dev API response cache.
type cache struct {
	FetchedAt time.Time   `json:"fetched_at"`
	Data      apiResponse `json:"data"`
}

// cachePath returns the path to the cache file.
func cachePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheFile), nil
}

// readCache loads the cache from disk. Returns nil if missing or corrupt.
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

// writeCache writes the cache to disk atomically.
func writeCache(c *cache) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install cache: %w", err)
	}
	return nil
}

// CacheStale returns true if the cache is missing or older than 24h.
func CacheStale() bool {
	c := readCache()
	if c == nil {
		return true
	}
	return time.Since(c.FetchedAt) > cacheMaxAge
}

// RefreshCache fetches from models.dev, updates the cache, regenerates
// models.yaml, and updates the in-memory registry. Designed to run in a
// background goroutine on startup.
func RefreshCache(ctx context.Context, registry *provider.Registry) error {
	data, err := fetch(ctx, apiURL)
	if err != nil {
		return fmt.Errorf("fetch models.dev: %w", err)
	}

	// Write cache
	if err := writeCache(&cache{FetchedAt: time.Now(), Data: data}); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}

	// Regenerate models.yaml from fresh data
	yaml := generateYAML(data)
	modPath, err := config.ModelsPath()
	if err != nil {
		return fmt.Errorf("models path: %w", err)
	}
	if err := writeModelsAtomic(modPath, yaml); err != nil {
		return fmt.Errorf("update models.yaml: %w", err)
	}

	// Update in-memory registry
	updateRegistryFromData(registry, data)

	return nil
}

// updateRegistryFromData updates the in-memory registry with fresh API data.
func updateRegistryFromData(registry *provider.Registry, data apiResponse) {
	existing := indexExisting(registry)
	for _, provName := range supportedProviders {
		prov, ok := data[provName]
		if !ok {
			continue
		}
		updateProviderModels(registry, existing, provName, prov)
	}
}

// writeModelsAtomic overwrites models.yaml atomically (always writes, unlike
// WriteModelsData which skips if the file exists).
func writeModelsAtomic(path, content string) error {
	return config.AtomicWrite(path, []byte(content), 0o644)
}
