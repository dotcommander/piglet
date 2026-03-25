package modelsdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupCacheDir points XDG_CONFIG_HOME at a fresh temp directory and creates
// the piglet subdirectory inside it. Returns the piglet config dir path.
// NOTE: t.Setenv is incompatible with t.Parallel — these tests must not be parallel.
func setupCacheDir(t *testing.T) string {
	t.Helper()
	xdg := t.TempDir()
	dir := filepath.Join(xdg, "piglet")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	t.Setenv("XDG_CONFIG_HOME", xdg)
	return dir
}

// sampleCache returns a cache value with one OpenAI model entry.
func sampleCache(fetchedAt time.Time) *cache {
	return &cache{
		FetchedAt: fetchedAt,
		Data: apiResponse{
			"openai": providerData{
				ID: "openai",
				Models: map[string]modelData{
					"gpt-4o": {
						ID:   "gpt-4o",
						Name: "GPT-4o",
						Limit: modelLimit{Context: 128000, Output: 16384},
					},
				},
			},
		},
	}
}

// writeCacheJSON marshals c and writes it directly to the cache file path (no atomic tmp).
func writeCacheJSON(t *testing.T, dir string, c *cache) {
	t.Helper()
	data, err := json.Marshal(c)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, cacheFile), data, 0o644))
}

// TestReadCache_MissingFile verifies readCache returns nil when no file exists.
func TestReadCache_MissingFile(t *testing.T) {
	setupCacheDir(t)
	assert.Nil(t, readCache())
}

// TestReadCache_CorruptFile verifies readCache returns nil on malformed JSON.
func TestReadCache_CorruptFile(t *testing.T) {
	dir := setupCacheDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, cacheFile), []byte("not json at all"), 0o644))
	assert.Nil(t, readCache())
}

// TestReadCache_ValidFile verifies readCache parses and returns a well-formed cache.
func TestReadCache_ValidFile(t *testing.T) {
	dir := setupCacheDir(t)
	want := sampleCache(time.Now().Truncate(time.Second).UTC())
	writeCacheJSON(t, dir, want)

	got := readCache()
	require.NotNil(t, got)
	assert.True(t, want.FetchedAt.Equal(got.FetchedAt), "FetchedAt mismatch: want %v got %v", want.FetchedAt, got.FetchedAt)
	assert.Equal(t, want.Data, got.Data)
}

// TestWriteCache_CreatesFile verifies writeCache persists a cache that readCache can round-trip.
func TestWriteCache_CreatesFile(t *testing.T) {
	setupCacheDir(t)
	want := sampleCache(time.Now().Truncate(time.Second).UTC())

	require.NoError(t, writeCache(want))

	got := readCache()
	require.NotNil(t, got)
	assert.True(t, want.FetchedAt.Equal(got.FetchedAt))
	assert.Equal(t, want.Data, got.Data)
}

// TestWriteCache_NoTmpFileRemains verifies atomic write leaves no .tmp file.
func TestWriteCache_NoTmpFileRemains(t *testing.T) {
	dir := setupCacheDir(t)
	require.NoError(t, writeCache(sampleCache(time.Now())))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, cacheFile+".tmp", e.Name(), "stale .tmp file found after atomic write")
	}
}

// TestCacheStale_NoCache verifies CacheStale returns true when no file exists.
func TestCacheStale_NoCache(t *testing.T) {
	setupCacheDir(t)
	assert.True(t, CacheStale())
}

// TestCacheStale_FreshCache verifies CacheStale returns false for a recent cache.
func TestCacheStale_FreshCache(t *testing.T) {
	dir := setupCacheDir(t)
	writeCacheJSON(t, dir, sampleCache(time.Now()))
	assert.False(t, CacheStale())
}

// TestCacheStale_StaleCache verifies CacheStale returns true when cache is older than 24h.
func TestCacheStale_StaleCache(t *testing.T) {
	dir := setupCacheDir(t)
	writeCacheJSON(t, dir, sampleCache(time.Now().Add(-25*time.Hour)))
	assert.True(t, CacheStale())
}
