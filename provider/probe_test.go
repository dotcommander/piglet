package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
