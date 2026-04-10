// Package cache provides a file-backed TTL cache for piglet extensions.
// Entries are stored as JSON files under ~/.config/piglet/cache/<namespace>/.
// Keys are hashed (sha256) to produce safe filenames.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

const defaultMaxEntries = 500

type entry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`
}

// resolve returns ~/.config/piglet/cache/<namespace>/ without creating it.
func resolve(namespace string) (string, error) {
	base, err := xdg.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("cache dir: %w", err)
	}
	return filepath.Join(base, "cache", namespace), nil
}

func keyFile(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:]) + ".json"
}

// Get retrieves a cached value. Returns ("", false) on miss or expiry.
func Get(namespace, key string) (string, bool) {
	d, err := resolve(namespace)
	if err != nil {
		return "", false
	}
	path := filepath.Join(d, keyFile(key))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return "", false
	}
	if !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt) {
		os.Remove(path) //nolint:errcheck
		return "", false
	}
	return e.Value, true
}

// Set stores value under namespace/key with the given TTL.
// TTL <= 0 means no expiry.
func Set(namespace, key, value string, ttl time.Duration) error {
	e := entry{Key: key, Value: value}
	if ttl > 0 {
		e.ExpiresAt = time.Now().Add(ttl)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("cache set marshal: %w", err)
	}
	d, err := resolve(namespace)
	if err != nil {
		return err
	}
	path := filepath.Join(d, keyFile(key))
	if err := xdg.WriteFileAtomic(path, data); err != nil {
		return fmt.Errorf("cache set: %w", err)
	}
	_ = GC(namespace, defaultMaxEntries)
	return nil
}

// Delete removes a single entry.
func Delete(namespace, key string) error {
	d, err := resolve(namespace)
	if err != nil {
		return err
	}
	path := filepath.Join(d, keyFile(key))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}

// Purge removes all entries in a namespace.
func Purge(namespace string) error {
	d, err := resolve(namespace)
	if err != nil {
		return fmt.Errorf("cache purge: %w", err)
	}
	if err := os.RemoveAll(d); err != nil {
		return fmt.Errorf("cache purge: %w", err)
	}
	return nil
}

// GC removes the oldest entries when count exceeds maxEntries.
func GC(namespace string, maxEntries int) error {
	d, err := resolve(namespace)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cache gc readdir: %w", err)
	}

	type fileInfo struct {
		name    string
		modTime time.Time
	}
	var files []fileInfo
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{name: e.Name(), modTime: info.ModTime()})
	}
	if len(files) <= maxEntries {
		return nil
	}
	slices.SortFunc(files, func(a, b fileInfo) int {
		return a.modTime.Compare(b.modTime)
	})
	toRemove := len(files) - maxEntries
	for i := range toRemove {
		path := filepath.Join(d, files[i].name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cache gc remove: %w", err)
		}
	}
	return nil
}
