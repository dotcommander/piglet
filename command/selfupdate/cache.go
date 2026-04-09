package selfupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dotcommander/piglet/config"
)

const (
	cacheFile   = ".update-check.json"
	cacheMaxAge = 24 * time.Hour
)

// ReleaseInfo holds the fields we care about from the GitHub releases API.
type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
}

type updateCache struct {
	CheckedAt time.Time   `json:"checked_at"`
	Release   ReleaseInfo `json:"release"`
}

// cacheDir is the directory used for the update cache. Empty string means
// use config.ConfigDir() at runtime. Tests override directly.
var cacheDir string

// resolveCachePath returns the full path to the cache file.
func resolveCachePath() (string, error) {
	dir := cacheDir
	if dir == "" {
		var err error
		dir, err = config.ConfigDir()
		if err != nil {
			return "", fmt.Errorf("selfupdate cache path: %w", err)
		}
	}
	return filepath.Join(dir, cacheFile), nil
}

// readCache reads the update cache from disk. Returns nil if missing or corrupt.
func readCache() *updateCache {
	path, err := resolveCachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c updateCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return &c
}

// CheckStale returns true if the update cache is missing, corrupt, or older
// than 24 hours.
func CheckStale() bool {
	c := readCache()
	if c == nil {
		return true
	}
	return time.Since(c.CheckedAt) > cacheMaxAge
}

// CachedRelease returns the cached release if the cache is fresh, otherwise
// returns a zero ReleaseInfo.
func CachedRelease() ReleaseInfo {
	c := readCache()
	if c == nil || time.Since(c.CheckedAt) > cacheMaxAge {
		return ReleaseInfo{}
	}
	return c.Release
}

// WriteCache writes a release to the update cache atomically.
func WriteCache(r ReleaseInfo) error {
	path, err := resolveCachePath()
	if err != nil {
		return err
	}

	c := updateCache{CheckedAt: time.Now(), Release: r}
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal update cache: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write update cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install update cache: %w", err)
	}
	return nil
}
