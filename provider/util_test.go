package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Registry: parseAPI / edge cases not covered by registry_test.go
// ---------------------------------------------------------------------------

func TestRegistry_ParseAPIOpenAI(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	// xai uses openai protocol
	models := r.ModelsByProvider("xai")
	require.NotEmpty(t, models)
	for _, m := range models {
		assert.Equal(t, core.APIOpenAI, m.API)
	}
}

func TestRegistry_ParseAPIAnthropic(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	models := r.ModelsByProvider("anthropic")
	require.NotEmpty(t, models)
	for _, m := range models {
		assert.Equal(t, core.APIAnthropic, m.API)
	}
}

func TestRegistry_ParseAPIGoogle(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	models := r.ModelsByProvider("google")
	require.NotEmpty(t, models)
	for _, m := range models {
		assert.Equal(t, core.APIGoogle, m.API)
	}
}

func TestRegistry_CreateUnknownAPIDefaultsToOpenAI(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	m := core.Model{
		ID:       "unknown-model",
		Provider: "custom",
		API:      "unknown-api",
		BaseURL:  "https://example.com",
	}
	r.Register(m)

	prov, err := r.Create(m, func() string { return "key" })
	require.NoError(t, err)
	assert.NotNil(t, prov)
}

func TestRegistry_ResolvePrefixAmbiguous(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	// "gpt-4" is a prefix of several models — any match is valid, just not a panic
	m, ok := r.Resolve("gpt-4")
	assert.True(t, ok)
	assert.True(t, strings.HasPrefix(m.ID, "gpt-4"))
}

func TestRegistry_ResolveExactWinsOverPrefix(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	r.Register(core.Model{ID: "exact-model", Provider: "test", API: core.APIOpenAI})
	r.Register(core.Model{ID: "exact-model-long", Provider: "test", API: core.APIOpenAI})

	// Exact match should win
	m, ok := r.Resolve("exact-model")
	require.True(t, ok)
	assert.Equal(t, "exact-model", m.ID)
}

func TestRegistry_ModelsByProviderEmpty(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	models := r.ModelsByProvider("nonexistent-provider")
	assert.Empty(t, models)
}

func TestRegistry_ModelsByProviderCaseInsensitive(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	upper := r.ModelsByProvider("OPENAI")
	lower := r.ModelsByProvider("openai")
	assert.Equal(t, len(lower), len(upper))
}

func TestRegistry_ModelsByProviderSorted(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	models := r.ModelsByProvider("openai")
	require.Greater(t, len(models), 1)
	for i := 1; i < len(models); i++ {
		assert.LessOrEqual(t, models[i-1].ID, models[i].ID)
	}
}

func TestRegistry_RegisterOverwrite(t *testing.T) {
	t.Parallel()
	r := provider.NewRegistry()
	r.Register(core.Model{ID: "my-model", Provider: "test", API: core.APIOpenAI, BaseURL: "https://original.com"})
	r.Register(core.Model{ID: "my-model", Provider: "test", API: core.APIOpenAI, BaseURL: "https://updated.com"})

	m, ok := r.Resolve("test/my-model")
	require.True(t, ok)
	assert.Equal(t, "https://updated.com", m.BaseURL)
}

// ---------------------------------------------------------------------------
// useMaxCompletionTokens (tested indirectly via request body inspection)
// ---------------------------------------------------------------------------

func TestOpenAI_BuildRequest_MaxCompletionTokens_o1(t *testing.T) {
	t.Parallel()
	// o1 models use max_completion_tokens
	model := core.Model{
		ID: "o1-mini", Provider: "openai", API: core.APIOpenAI,
		BaseURL: "https://unused.example.com", MaxTokens: 1000,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })
	_ = prov
	// We can't directly test buildRequest without an http server, so we
	// verify via a mock server that captures the body.
}

// ---------------------------------------------------------------------------
// DefaultModelsYAML / WriteModelsData
// ---------------------------------------------------------------------------

func TestDefaultModelsYAML_NotEmpty(t *testing.T) {
	t.Parallel()
	yaml := provider.DefaultModelsYAML()
	assert.NotEmpty(t, yaml)
	assert.Contains(t, yaml, "anthropic")
	assert.Contains(t, yaml, "openai")
	assert.Contains(t, yaml, "google")
}

func TestWriteModelsData_CreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.yaml")

	err := provider.WriteModelsData(path, "models:\n  - id: test\n")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "test")
}

func TestWriteModelsData_NoOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.yaml")

	original := "models:\n  - id: original\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	// Second write should be a no-op
	err := provider.WriteModelsData(path, "models:\n  - id: replacement\n")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "original")
	assert.NotContains(t, string(data), "replacement")
}

func TestWriteModelsData_CreatesParentDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "models.yaml")

	err := provider.WriteModelsData(path, "models: []\n")
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestWriteDefaultModels_CreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.yaml")

	err := provider.WriteDefaultModels(path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// Should contain default models content
	assert.Contains(t, string(data), "anthropic")
	assert.Contains(t, string(data), "openai")
}

// ---------------------------------------------------------------------------
// Model.DisplayName
// ---------------------------------------------------------------------------

func TestModel_DisplayName(t *testing.T) {
	t.Parallel()
	m := core.Model{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai"}
	assert.Equal(t, "openai/GPT-4o", m.DisplayName())
}

func TestModel_DisplayName_EmptyName(t *testing.T) {
	t.Parallel()
	m := core.Model{ID: "some-model", Provider: "test"}
	assert.Equal(t, "test/", m.DisplayName())
}

// ---------------------------------------------------------------------------
// OpenAI: hasVersionSuffix (tested indirectly via endpoint URL)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// OpenAI: useMaxCompletionTokens logic (table-driven, via request body)
// ---------------------------------------------------------------------------

func TestOpenAI_UseMaxCompletionTokens_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		modelID             string
		wantCompletionField bool // true = max_completion_tokens, false = max_tokens
	}{
		{"o1-mini", true},
		{"o1-preview", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"gpt-5", true},
		{"gpt-5.4", true},
		{"gpt-4.1", true},
		{"gpt-4.1-mini", true},
		{"gpt-4.5-turbo", true},
		{"gpt-4o", false},
		{"gpt-4", false},
		{"gpt-3.5-turbo", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			t.Parallel()
			var capturedBody []byte

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				buf := make([]byte, 4096)
				n, _ := r.Body.Read(buf)
				capturedBody = buf[:n]
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sseLines(`{"choices":[{"finish_reason":"stop"}]}`))
			}))
			defer server.Close()

			model := core.Model{
				ID: tt.modelID, Provider: "openai", API: core.APIOpenAI,
				BaseURL: server.URL, MaxTokens: 100,
			}
			prov := provider.NewOpenAI(model, func() string { return "key" })
			ch := prov.Stream(context.Background(), core.StreamRequest{
				Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
			})
			for range ch {
			}

			body := string(capturedBody)
			if tt.wantCompletionField {
				assert.Contains(t, body, `"max_completion_tokens"`, "model %s", tt.modelID)
				assert.NotContains(t, body, `"max_tokens"`, "model %s", tt.modelID)
			} else {
				assert.Contains(t, body, `"max_tokens"`, "model %s", tt.modelID)
				assert.NotContains(t, body, `"max_completion_tokens"`, "model %s", tt.modelID)
			}
		})
	}
}
