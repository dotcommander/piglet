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
