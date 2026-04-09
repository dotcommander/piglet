package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanLocalServers_NoServers(t *testing.T) {
	t.Parallel()
	result, err := ScanLocalServers()
	if err != nil {
		// No local server running — expected in CI.
		assert.Contains(t, err.Error(), "no local model server found")
		assert.Empty(t, result.URL)
	} else {
		// A local server happened to be running — scan should return a URL.
		assert.NotEmpty(t, result.URL)
	}
}

func TestProbeServer_SkipsEmbedding(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "text-embedding-3-large", "owned_by": "test"},
				{"id": "llama-3.3-70b", "owned_by": "test"},
			},
		})
	}))
	defer srv.Close()

	result, err := ProbeServer(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "llama-3.3-70b", result.ModelID)
}

func TestProbeServer_LlamaCppRouterStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "mistral-7b.gguf", "owned_by": "llamacpp", "status": map[string]string{"value": "unloaded"}},
				{"id": "llama-3.3-8b.gguf", "owned_by": "llamacpp", "status": map[string]string{"value": "loaded"}},
			},
		})
	}))
	defer srv.Close()

	result, err := ProbeServer(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "llama-3.3-8b.gguf", result.ModelID)
	assert.Equal(t, "loaded", result.State)
	assert.Equal(t, "llamacpp", result.ServerType)
}

func TestProbeServer_OllamaPS(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "llama3.2", "owned_by": "ollama"},
					{"id": "qwen2.5-coder:32b", "owned_by": "ollama"},
					{"id": "nomic-embed-text", "owned_by": "ollama"},
				},
			})
		case "/api/ps":
			json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{
					{"name": "qwen2.5-coder:32b"},
				},
			})
		}
	}))
	defer srv.Close()

	result, err := ProbeServer(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "qwen2.5-coder:32b", result.ModelID)
	assert.Equal(t, "loaded", result.State)
}

func TestProbeServer_PrefersLoaded(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "qwen2.5-coder-32b", "owned_by": "lmstudio", "state": "not-loaded"},
				{"id": "deepseek-r1:8b", "owned_by": "lmstudio", "state": "not-loaded"},
				{"id": "llama-3.3-70b", "owned_by": "lmstudio", "state": "loaded"},
			},
		})
	}))
	defer srv.Close()

	result, err := ProbeServer(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "llama-3.3-70b", result.ModelID)
	assert.Equal(t, "loaded", result.State)
}

func TestProbeServer_SingleModel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "qwen3-30b-a3b", "owned_by": "mlx"},
			},
		})
	}))
	defer srv.Close()

	result, err := ProbeServer(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "qwen3-30b-a3b", result.ModelID)
}

func TestBestModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []ProbeResult
		want    string
	}{
		{"empty", nil, ""},
		{"single chat", []ProbeResult{{ModelID: "llama-3.3-70b"}}, "llama-3.3-70b"},
		{"skip embedding first", []ProbeResult{{ModelID: "text-embedding-3-large"}, {ModelID: "llama-3.3-70b"}}, "llama-3.3-70b"},
		{"all embedding falls back to first", []ProbeResult{{ModelID: "text-embedding-3-large"}}, "text-embedding-3-large"},
		{"prefer loaded over unloaded", []ProbeResult{
			{ModelID: "qwen2.5-coder-32b", State: "not-loaded"},
			{ModelID: "llama-3.3-70b", State: "loaded"},
		}, "llama-3.3-70b"},
		{"loaded embedding skipped for loaded chat", []ProbeResult{
			{ModelID: "text-embedding-3-large", State: "loaded"},
			{ModelID: "qwen2.5-coder-32b", State: "not-loaded"},
			{ModelID: "llama-3.3-70b", State: "loaded"},
		}, "llama-3.3-70b"},
		{"no state falls back to first non-embedding", []ProbeResult{
			{ModelID: "qwen2.5-coder-32b"},
			{ModelID: "llama-3.3-70b"},
		}, "qwen2.5-coder-32b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, BestModel(tt.results))
		})
	}
}

func TestIsEmbeddingModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id   string
		want bool
	}{
		{"KnightsAnalytics/all-MiniLM-L6-v2", true},
		{"text-embedding-3-large", true},
		{"nomic-embed-text-v1.5", true},
		{"BAAI/bge-large-en-v1.5", true},
		{"intfloat/e5-mistral-7b-instruct", true},
		{"mxbai-embed-large", true},
		{"snowflake-arctic-embed-m", true},
		{"hkunlp/instructor-large", true},
		{"Alibaba-NLP/gte-Qwen2-7B-instruct", true},

		{"mlx-community/Qwen3.5-9B-MLX-4bit", false},
		{"qwen2.5-coder-32b", false},
		{"deepseek-r1:8b", false},
		{"llama-3.3-70b-versatile", false},
		{"gpt-4o", false},
		{"claude-sonnet-4-20250514", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isEmbeddingModel(tt.id))
		})
	}
}
