package provider_test

import (
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/provider"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_BuiltinModels(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	models := r.Models()
	assert.Greater(t, len(models), 5)
}

func TestRegistry_ResolveExact(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()

	m, ok := r.Resolve("openai/gpt-4o")
	require.True(t, ok)
	assert.Equal(t, "gpt-4o", m.ID)
	assert.Equal(t, "openai", m.Provider)
}

func TestRegistry_ResolveByID(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()

	m, ok := r.Resolve("gpt-4o")
	require.True(t, ok)
	assert.Equal(t, "gpt-4o", m.ID)
}

func TestRegistry_ResolvePrefix(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	r.Register(core.Model{
		ID:       "test-prefix-model",
		Provider: "testprov",
		API:      core.APIOpenAI,
	})

	m, ok := r.Resolve("test-prefix-m")
	require.True(t, ok)
	assert.Equal(t, "test-prefix-model", m.ID)
}

func TestRegistry_ResolveNotFound(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()

	_, ok := r.Resolve("nonexistent-model")
	assert.False(t, ok)
}

func TestRegistry_RegisterCustom(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()

	custom := core.Model{
		ID:       "custom-1",
		Name:     "Custom Model",
		Provider: "myhost",
		API:      core.APIOpenAI,
		BaseURL:  "https://myhost.com",
	}
	r.Register(custom)

	m, ok := r.Resolve("myhost/custom-1")
	require.True(t, ok)
	assert.Equal(t, "custom-1", m.ID)
}

func TestRegistry_ModelsByProvider(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()

	models := r.ModelsByProvider("openai")
	assert.Greater(t, len(models), 0)
	for _, m := range models {
		assert.Equal(t, "openai", m.Provider)
	}
}

func TestRegistry_CreateOpenAI(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()

	m, ok := r.Resolve("gpt-4o")
	require.True(t, ok)

	prov, err := r.Create(m, func() string { return "sk-test" })
	require.NoError(t, err)
	assert.NotNil(t, prov)
}

func TestRegistry_ModelsSorted(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	models := r.Models()

	for i := 1; i < len(models); i++ {
		prev := models[i-1]
		curr := models[i]
		if prev.Provider == curr.Provider {
			assert.LessOrEqual(t, prev.ID, curr.ID, "models should be sorted by ID within provider")
		} else {
			assert.Less(t, prev.Provider, curr.Provider, "models should be sorted by provider")
		}
	}
}

func TestRegistry_CreateAllAPIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modelID string
		wantAPI core.API
	}{
		{"openai", "gpt-4o", core.APIOpenAI},
		{"anthropic", "claude-sonnet-4-20250514", core.APIAnthropic},
		{"google", "gemini-2.5-pro", core.APIGoogle},
	}

	r := provider.NewRegistry()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, ok := r.Resolve(tt.modelID)
			require.True(t, ok, "model %q not found", tt.modelID)
			assert.Equal(t, tt.wantAPI, m.API)

			prov, err := r.Create(m, func() string { return "sk-test" })
			require.NoError(t, err)
			assert.NotNil(t, prov)
		})
	}
}

func TestRegistry_ResolveAnthropicModels(t *testing.T) {
	t.Parallel()

	r := provider.NewRegistry()
	models := r.ModelsByProvider("anthropic")
	require.NotEmpty(t, models, "anthropic provider should have models")

	for _, m := range models {
		t.Run(m.ID, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "anthropic", m.Provider)
			assert.Equal(t, core.APIAnthropic, m.API)
			assert.NotEmpty(t, m.BaseURL)
			assert.Greater(t, m.MaxTokens, 0)

			resolved, ok := r.Resolve(m.ID)
			require.True(t, ok)
			assert.Equal(t, m.ID, resolved.ID)
		})
	}
}

func TestRegistry_ResolveGoogleModels(t *testing.T) {
	t.Parallel()

	r := provider.NewRegistry()
	models := r.ModelsByProvider("google")
	require.NotEmpty(t, models, "google provider should have models")

	for _, m := range models {
		t.Run(m.ID, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "google", m.Provider)
			assert.Equal(t, core.APIGoogle, m.API)
			assert.NotEmpty(t, m.BaseURL)
			assert.Greater(t, m.MaxTokens, 0)

			resolved, ok := r.Resolve(m.ID)
			require.True(t, ok)
			assert.Equal(t, m.ID, resolved.ID)
		})
	}
}

func TestRegistry_ResolveCaseInsensitive(t *testing.T) {
	t.Parallel()

	r := provider.NewRegistry()

	tests := []struct {
		query        string
		wantProvider string
	}{
		{"openai/gpt-4o", "openai"},
		{"ANTHROPIC/claude-sonnet-4-20250514", "anthropic"},
		{"GOOGLE/gemini-2.5-pro", "google"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()
			m, ok := r.Resolve(tt.query)
			require.True(t, ok, "Resolve(%q) returned false", tt.query)
			assert.Equal(t, tt.wantProvider, m.Provider)
		})
	}
}
