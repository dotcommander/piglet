package modelsdev_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/modelsdev"
	"github.com/dotcommander/piglet/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestServer creates an httptest.Server returning the given API response JSON.
func buildTestServer(t *testing.T, payload any) *httptest.Server {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
}

// authWith returns an auth that has keys set for the given providers.
func authWith(providers ...string) *config.Auth {
	a := config.NewAuth("")
	for _, p := range providers {
		_ = a.SetKey(p, "test-key")
	}
	return a
}

func TestSync_UpdatesExistingModels(t *testing.T) {
	t.Parallel()

	// gpt-4o is a builtin with ContextWindow=128000, MaxTokens=16384.
	// Provide new limits from models.dev.
	payload := map[string]any{
		"openai": map[string]any{
			"id": "openai",
			"models": map[string]any{
				"gpt-4o": map[string]any{
					"id":   "gpt-4o",
					"name": "GPT-4o",
					"limit": map[string]any{
						"context": 256000,
						"output":  32768,
					},
				},
			},
		},
	}

	srv := buildTestServer(t, payload)
	defer srv.Close()

	reg := provider.NewRegistry()
	auth := authWith("openai")

	updated, err := modelsdev.SyncFromURL(context.Background(), srv.URL, reg, auth)
	require.NoError(t, err)
	assert.Equal(t, 1, updated)

	m, ok := reg.Resolve("openai/gpt-4o")
	require.True(t, ok)
	assert.Equal(t, 256000, m.ContextWindow)
	assert.Equal(t, 32768, m.MaxTokens)
}

func TestSync_DoesNotAddNewModels(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"openai": map[string]any{
			"id": "openai",
			"models": map[string]any{
				"gpt-5-turbo": map[string]any{
					"id":   "gpt-5-turbo",
					"name": "GPT-5 Turbo",
					"limit": map[string]any{
						"context": 500000,
						"output":  100000,
					},
				},
			},
		},
	}

	srv := buildTestServer(t, payload)
	defer srv.Close()

	reg := provider.NewRegistry()
	initialCount := len(reg.Models())
	auth := authWith("openai")

	updated, err := modelsdev.SyncFromURL(context.Background(), srv.URL, reg, auth)
	require.NoError(t, err)
	assert.Equal(t, 0, updated)
	assert.Equal(t, initialCount, len(reg.Models()), "no new models should be added")

	_, found := reg.Resolve("openai/gpt-5-turbo")
	assert.False(t, found, "new model should not be registered")
}

func TestSync_SkipsProvidersWithoutAuth(t *testing.T) {
	t.Parallel()

	// Both providers have updated models, but only openai has auth.
	payload := map[string]any{
		"openai": map[string]any{
			"id": "openai",
			"models": map[string]any{
				"gpt-4o": map[string]any{
					"id":    "gpt-4o",
					"name":  "GPT-4o",
					"limit": map[string]any{"context": 256000, "output": 32768},
				},
			},
		},
		"anthropic": map[string]any{
			"id": "anthropic",
			"models": map[string]any{
				"claude-sonnet-4-20250514": map[string]any{
					"id":    "claude-sonnet-4-20250514",
					"name":  "Claude Sonnet 4",
					"limit": map[string]any{"context": 300000, "output": 16384},
				},
			},
		},
	}

	srv := buildTestServer(t, payload)
	defer srv.Close()

	reg := provider.NewRegistry()
	auth := authWith("openai") // no anthropic auth

	updated, err := modelsdev.SyncFromURL(context.Background(), srv.URL, reg, auth)
	require.NoError(t, err)
	assert.Equal(t, 1, updated, "only openai model should be updated")

	// openai model should be updated
	m, ok := reg.Resolve("openai/gpt-4o")
	require.True(t, ok)
	assert.Equal(t, 256000, m.ContextWindow)

	// anthropic model should keep original values
	m2, ok := reg.Resolve("anthropic/claude-sonnet-4-20250514")
	require.True(t, ok)
	assert.Equal(t, 200000, m2.ContextWindow, "anthropic model should not be updated")
}

func TestSync_NoUpdateWhenUnchanged(t *testing.T) {
	t.Parallel()

	// Provide identical values to builtins — no update expected.
	payload := map[string]any{
		"openai": map[string]any{
			"id": "openai",
			"models": map[string]any{
				"gpt-4o": map[string]any{
					"id":    "gpt-4o",
					"name":  "GPT-4o",
					"limit": map[string]any{"context": 128000, "output": 16384},
				},
			},
		},
	}

	srv := buildTestServer(t, payload)
	defer srv.Close()

	reg := provider.NewRegistry()
	auth := authWith("openai")

	updated, err := modelsdev.SyncFromURL(context.Background(), srv.URL, reg, auth)
	require.NoError(t, err)
	assert.Equal(t, 0, updated, "no change means no update")
}

func TestSync_UnsupportedProvidersIgnored(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"cohere": map[string]any{
			"id": "cohere",
			"models": map[string]any{
				"command-r": map[string]any{
					"id":    "command-r",
					"name":  "Command R",
					"limit": map[string]any{"context": 128000, "output": 4096},
				},
			},
		},
	}

	srv := buildTestServer(t, payload)
	defer srv.Close()

	reg := provider.NewRegistry()
	// Even with auth for cohere, it's not in providerAliases
	auth := authWith("cohere")

	updated, err := modelsdev.SyncFromURL(context.Background(), srv.URL, reg, auth)
	require.NoError(t, err)
	assert.Equal(t, 0, updated)
}

func TestSync_UpdatesNameOnly(t *testing.T) {
	t.Parallel()

	// Update only the display name, keep same limits.
	payload := map[string]any{
		"openai": map[string]any{
			"id": "openai",
			"models": map[string]any{
				"gpt-4o": map[string]any{
					"id":    "gpt-4o",
					"name":  "GPT-4o (2026)",
					"limit": map[string]any{"context": 128000, "output": 16384},
				},
			},
		},
	}

	srv := buildTestServer(t, payload)
	defer srv.Close()

	reg := provider.NewRegistry()
	auth := authWith("openai")

	updated, err := modelsdev.SyncFromURL(context.Background(), srv.URL, reg, auth)
	require.NoError(t, err)
	assert.Equal(t, 1, updated)

	m, ok := reg.Resolve("openai/gpt-4o")
	require.True(t, ok)
	assert.Equal(t, "GPT-4o (2026)", m.Name)
}
