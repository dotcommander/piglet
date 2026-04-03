package modelsdev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchOverrides_BuildsCorrectKeys(t *testing.T) {
	apiResp := apiResponse{
		"openai": providerData{ID: "openai", Models: map[string]modelData{
			"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o", Limit: modelLimit{Context: 128000, Output: 16384}},
		}},
		"anthropic": providerData{ID: "anthropic", Models: map[string]modelData{
			"claude-sonnet-4-20250514": {ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Limit: modelLimit{Context: 200000, Output: 64000}},
		}},
	}
	srv := serveJSON(t, apiResp)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "piglet"), 0o755))

	overrides, err := fetchOverridesFromURL(context.Background(), srv.URL)
	require.NoError(t, err)

	ov, ok := overrides["openai/gpt-4o"]
	require.True(t, ok)
	assert.Equal(t, "GPT-4o", ov.Name)
	assert.Equal(t, 128000, ov.ContextWindow)
	assert.Equal(t, 16384, ov.MaxTokens)

	ov, ok = overrides["anthropic/claude-sonnet-4-20250514"]
	require.True(t, ok)
	assert.Equal(t, "Claude Sonnet 4", ov.Name)
	assert.Equal(t, 200000, ov.ContextWindow)
	assert.Equal(t, 64000, ov.MaxTokens)
}

func TestFetchOverrides_WritesCache(t *testing.T) {
	apiResp := apiResponse{
		"openai": providerData{ID: "openai", Models: map[string]modelData{
			"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o", Limit: modelLimit{Context: 128000, Output: 16384}},
		}},
	}
	srv := serveJSON(t, apiResp)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "piglet"), 0o755))

	_, err := fetchOverridesFromURL(context.Background(), srv.URL)
	require.NoError(t, err)

	assert.False(t, CacheStale(), "cache should be fresh after fetch")
}

func TestCacheStale_Missing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	assert.True(t, CacheStale())
}

func TestCacheStale_Fresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	pigletDir := filepath.Join(dir, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0o755))

	c := &cache{FetchedAt: time.Now(), Data: apiResponse{}}
	data, _ := json.Marshal(c)
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, cacheFile), data, 0o644))

	assert.False(t, CacheStale())
}

func TestCacheStale_Expired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	pigletDir := filepath.Join(dir, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0o755))

	c := &cache{FetchedAt: time.Now().Add(-25 * time.Hour), Data: apiResponse{}}
	data, _ := json.Marshal(c)
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, cacheFile), data, 0o644))

	assert.True(t, CacheStale())
}

func serveJSON(t *testing.T, v any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}))
	t.Cleanup(srv.Close)
	return srv
}
