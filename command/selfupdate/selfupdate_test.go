package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cacheDirMu serializes all tests that mutate the package-level cacheDir.
var cacheDirMu sync.Mutex

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{"equal", "v1.0.0", "v1.0.0", 0},
		{"newer latest", "v1.1.0", "v1.0.0", 1},
		{"older current", "v1.0.0", "v1.1.0", -1},
		{"dev always older", "dev", "v1.0.0", -1},
		{"dev-abc123 older", "dev-abc123", "v1.0.0", -1},
		{"dev-abc-dirty older", "dev-abc-dirty", "v1.0.0", -1},
		{"no v prefix", "1.0.0", "1.0.0", 0},
		{"mismatched lengths shorter current", "v1.0", "v1.0.0", 0},
		{"mismatched lengths shorter latest", "v1.0.0", "v1.0", 0},
		{"patch older", "v1.0.0", "v1.0.1", -1},
		{"patch newer", "v1.0.1", "v1.0.0", 1},
		{"major bump", "v2.0.0", "v1.9.9", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CompareVersions(tc.current, tc.latest)
			assert.Equal(t, tc.want, got)
		})
	}
}

// withCacheDir acquires cacheDirMu, sets the cache directory, and registers
// a cleanup to release the lock and reset to the default. Tests using this
// helper must NOT call t.Parallel() — mutual exclusion is provided by the
// mutex rather than test isolation.
func withCacheDir(t *testing.T, dir string) {
	t.Helper()
	cacheDirMu.Lock()
	cacheDir = dir
	t.Cleanup(func() {
		cacheDir = ""
		cacheDirMu.Unlock()
	})
}

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	withCacheDir(t, dir)

	// Before writing, cache should be stale and release zero.
	assert.True(t, CheckStale())
	assert.Equal(t, ReleaseInfo{}, CachedRelease())

	r := ReleaseInfo{
		TagName:     "v1.2.3",
		PublishedAt: time.Now().UTC().Truncate(time.Second),
		HTMLURL:     "https://github.com/dotcommander/piglet/releases/tag/v1.2.3",
	}
	require.NoError(t, WriteCache(r))

	assert.False(t, CheckStale())
	got := CachedRelease()
	assert.Equal(t, r.TagName, got.TagName)
	assert.Equal(t, r.HTMLURL, got.HTMLURL)
}

func TestCacheStaleAfterExpiry(t *testing.T) {
	dir := t.TempDir()
	withCacheDir(t, dir)

	// Write a cache with a CheckedAt in the past.
	old := updateCache{
		CheckedAt: time.Now().Add(-25 * time.Hour),
		Release:   ReleaseInfo{TagName: "v0.9.0"},
	}
	data, err := json.Marshal(old)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, cacheFile), data, 0o644))

	assert.True(t, CheckStale())
	assert.Equal(t, ReleaseInfo{}, CachedRelease())
}

// TestUpdateNotice runs subtests sequentially (not parallel) because they
// share the package-level cacheDir override.
func TestUpdateNotice(t *testing.T) {
	t.Parallel()

	t.Run("no cache returns empty", func(t *testing.T) {
		withCacheDir(t, t.TempDir())
		assert.Equal(t, "", UpdateNotice("v1.0.0"))
	})

	t.Run("newer available", func(t *testing.T) {
		withCacheDir(t, t.TempDir())
		require.NoError(t, WriteCache(ReleaseInfo{TagName: "v1.2.0"}))
		notice := UpdateNotice("v1.0.0")
		assert.Contains(t, notice, "v1.2.0")
		assert.Contains(t, notice, "v1.0.0")
		assert.Contains(t, notice, "piglet update")
	})

	t.Run("up to date returns empty", func(t *testing.T) {
		withCacheDir(t, t.TempDir())
		require.NoError(t, WriteCache(ReleaseInfo{TagName: "v1.0.0"}))
		assert.Equal(t, "", UpdateNotice("v1.0.0"))
	})

	t.Run("current newer than cached returns empty", func(t *testing.T) {
		withCacheDir(t, t.TempDir())
		require.NoError(t, WriteCache(ReleaseInfo{TagName: "v0.9.0"}))
		assert.Equal(t, "", UpdateNotice("v1.0.0"))
	})
}
