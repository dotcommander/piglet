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
	m := core.Model{ID: "gpt-5", Name: "GPT-5", Provider: "openai"}
	assert.Equal(t, "openai/GPT-5", m.DisplayName())
}

func TestModel_DisplayName_EmptyName(t *testing.T) {
	t.Parallel()
	m := core.Model{ID: "some-model", Provider: "test"}
	assert.Equal(t, "test/", m.DisplayName())
}

// ---------------------------------------------------------------------------
// OpenAI: endpoint URL construction (hasVersionSuffix)
// ---------------------------------------------------------------------------

func TestOpenAI_Endpoint_NoVersionSuffix(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(`{"choices":[{"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})
	for range ch {
	}

	assert.Equal(t, "/v1/chat/completions", gotPath)
}

func TestOpenAI_Endpoint_WithVersionSuffix(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(`{"choices":[{"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	// Base URL already ends with /v1 — should not double-add /v1
	model := core.Model{
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL + "/v1", MaxTokens: 100,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})
	for range ch {
	}

	assert.Equal(t, "/v1/chat/completions", gotPath)
}

// ---------------------------------------------------------------------------
// OpenAI: useMaxCompletionTokens logic (table-driven, via request body)
// o-series and newer gpt models use max_completion_tokens
// ---------------------------------------------------------------------------

func TestOpenAI_UseMaxCompletionTokens_OSeriesModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		modelID string
	}{
		{"o1-mini"},
		{"o1-preview"},
		{"o3-mini"},
		{"o4-mini"},
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
			assert.Contains(t, body, `"max_completion_tokens"`, "model %s should use max_completion_tokens", tt.modelID)
			assert.NotContains(t, body, `"max_tokens"`, "model %s should not use max_tokens", tt.modelID)
		})
	}
}

func TestOpenAI_UseMaxCompletionTokens_NewerGPTModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		modelID string
	}{
		{"gpt-5"},
		{"gpt-5.4"},
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
			assert.Contains(t, body, `"max_completion_tokens"`, "model %s should use max_completion_tokens", tt.modelID)
			assert.NotContains(t, body, `"max_tokens"`, "model %s should not use max_tokens", tt.modelID)
		})
	}
}

// ---------------------------------------------------------------------------
// OpenAI: request body shape
// ---------------------------------------------------------------------------

func TestOpenAI_BuildRequest_SystemMessage(t *testing.T) {
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
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })
	ch := prov.Stream(context.Background(), core.StreamRequest{
		System:   "You are an expert.",
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"system"`)
	assert.Contains(t, body, `"You are an expert."`)
	assert.Contains(t, body, `"stream":true`)
	assert.Contains(t, body, `"include_usage":true`)
}

func TestOpenAI_BuildRequest_ToolChoice(t *testing.T) {
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
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
		Tools:    []core.ToolSchema{{Name: "my_tool", Description: "does something"}},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"tool_choice":"auto"`)
	assert.Contains(t, body, `"my_tool"`)
	assert.Contains(t, body, `"function"`)
}

func TestOpenAI_BuildRequest_ImageBlock(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		capturedBody = buf[:n]
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(`{"choices":[{"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{
				Content: "describe this image",
				Blocks: []core.ContentBlock{
					core.ImageContent{MimeType: "image/jpeg", Data: "base64data"},
				},
				Timestamp: time.Now(),
			},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"image_url"`)
	assert.Contains(t, body, `"data:image/jpeg;base64,base64data"`)
}

func TestOpenAI_BuildRequest_ToolResultMessage(t *testing.T) {
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
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })

	now := time.Now()
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{Content: "call the tool", Timestamp: now},
			&core.AssistantMessage{
				Content: []core.AssistantContent{core.ToolCall{
					ID: "call_abc", Name: "echo", Arguments: map[string]any{"text": "hello"},
				}},
				StopReason: core.StopReasonTool,
				Timestamp:  now,
			},
			&core.ToolResultMessage{
				ToolCallID: "call_abc",
				ToolName:   "echo",
				Content:    []core.ContentBlock{core.TextContent{Text: "tool result"}},
				Timestamp:  now,
			},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"tool"`)
	assert.Contains(t, body, `"call_abc"`)
	assert.Contains(t, body, `"tool result"`)
}

func TestOpenAI_StreamCachedTokens(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":50,"completion_tokens":10,"total_tokens":60,"prompt_tokens_details":{"cached_tokens":30}}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})

	var finalMsg *core.AssistantMessage
	for evt := range ch {
		if evt.Type == core.StreamDone {
			finalMsg = evt.Message
		}
	}

	require.NotNil(t, finalMsg)
	assert.Equal(t, 50, finalMsg.Usage.InputTokens)
	assert.Equal(t, 10, finalMsg.Usage.OutputTokens)
	assert.Equal(t, 30, finalMsg.Usage.CacheReadTokens)
}

func TestOpenAI_StreamAssistantHistory(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		capturedBody = buf[:n]
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(`{"choices":[{"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })

	now := time.Now()
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{Content: "first", Timestamp: now},
			&core.AssistantMessage{
				Content:    []core.AssistantContent{core.TextContent{Text: "response text"}},
				StopReason: core.StopReasonStop,
				Timestamp:  now,
			},
			&core.UserMessage{Content: "second", Timestamp: now},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"assistant"`)
	assert.Contains(t, body, `"response text"`)
}

func TestOpenAI_StreamCustomHeaders(t *testing.T) {
	t.Parallel()

	var capturedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-Custom-Header")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(`{"choices":[{"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gpt-5", Provider: "openai", API: core.APIOpenAI,
		BaseURL: server.URL, MaxTokens: 100,
		Headers: map[string]string{"X-Custom-Header": "custom-value"},
	}
	prov := provider.NewOpenAI(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})
	for range ch {
	}

	assert.Equal(t, "custom-value", capturedHeader)
}
