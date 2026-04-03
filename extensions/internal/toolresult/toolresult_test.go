package toolresult

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		details   any
		wantText  string
		wantFound bool
	}{
		{
			name:      "plain string",
			details:   "hello world",
			wantText:  "hello world",
			wantFound: true,
		},
		{
			name:      "empty string",
			details:   "",
			wantText:  "",
			wantFound: true,
		},
		{
			name:      "nil input",
			details:   nil,
			wantText:  "",
			wantFound: false,
		},
		{
			name: "map with content as string",
			details: map[string]any{
				"content": "content value",
			},
			wantText:  "content value",
			wantFound: true,
		},
		{
			name: "map with content as block array single block",
			details: map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "block text"},
				},
			},
			wantText:  "block text",
			wantFound: true,
		},
		{
			name: "map with content as block array multiple blocks",
			details: map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "first block"},
					map[string]any{"type": "text", "text": "second block"},
				},
			},
			wantText:  "first block",
			wantFound: true,
		},
		{
			name: "map with content as block array with non-text first block",
			details: map[string]any{
				"content": []any{
					map[string]any{"type": "image", "data": "base64..."},
					map[string]any{"type": "text", "text": "second block"},
				},
			},
			wantText:  "second block",
			wantFound: true,
		},
		{
			name: "map with output key",
			details: map[string]any{
				"output": "output value",
			},
			wantText:  "output value",
			wantFound: true,
		},
		{
			name: "map with empty content string",
			details: map[string]any{
				"content": "",
			},
			wantText:  "",
			wantFound: true,
		},
		{
			name: "map with empty output string",
			details: map[string]any{
				"output": "",
			},
			wantText:  "",
			wantFound: true,
		},
		{
			name: "map with content as empty block array",
			details: map[string]any{
				"content": []any{},
			},
			wantText:  "",
			wantFound: false,
		},
		{
			name: "map with content as non-string non-slice",
			details: map[string]any{
				"content": 42,
			},
			wantText:  "",
			wantFound: false,
		},
		{
			name: "map with neither content nor output",
			details: map[string]any{
				"other": "value",
			},
			wantText:  "",
			wantFound: false,
		},
		{
			name: "map with output as non-string",
			details: map[string]any{
				"output": 123,
			},
			wantText:  "",
			wantFound: false,
		},
		{
			name:      "integer input",
			details:   42,
			wantText:  "",
			wantFound: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ExtractText(tc.details)
			assert.Equal(t, tc.wantFound, ok)
			assert.Equal(t, tc.wantText, got)
		})
	}
}

func TestReplaceText(t *testing.T) {
	t.Parallel()

	const replacement = "REPLACED"

	tests := []struct {
		name     string
		details  any
		wantType string // "string", "map"
	}{
		{
			name:     "plain string becomes replacement",
			details:  "original text",
			wantType: "string",
		},
		{
			name:     "empty string becomes replacement",
			details:  "",
			wantType: "string",
		},
		{
			name:     "nil becomes replacement string",
			details:  nil,
			wantType: "string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ReplaceText(tc.details, replacement)
			s, ok := got.(string)
			require.True(t, ok, "expected string result")
			assert.Equal(t, replacement, s)
		})
	}

	t.Run("map with content as string", func(t *testing.T) {
		t.Parallel()
		details := map[string]any{
			"content": "original",
			"extra":   "preserved",
		}
		got := ReplaceText(details, replacement)
		m, ok := got.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, replacement, m["content"])
		assert.Equal(t, "preserved", m["extra"])
		// original must not be mutated
		assert.Equal(t, "original", details["content"])
	})

	t.Run("map with content as block array replaces first block text", func(t *testing.T) {
		t.Parallel()
		original := map[string]any{"type": "text", "text": "original text"}
		second := map[string]any{"type": "text", "text": "second block"}
		details := map[string]any{
			"content": []any{original, second},
		}
		got := ReplaceText(details, replacement)
		m, ok := got.(map[string]any)
		require.True(t, ok)
		blocks, ok := m["content"].([]any)
		require.True(t, ok)
		require.Len(t, blocks, 2)
		first, ok := blocks[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, replacement, first["text"])
		// second block unchanged
		second2, ok := blocks[1].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "second block", second2["text"])
		// original map not mutated
		assert.Equal(t, "original text", original["text"])
	})

	t.Run("map with output key", func(t *testing.T) {
		t.Parallel()
		details := map[string]any{
			"output": "original output",
			"meta":   "stays",
		}
		got := ReplaceText(details, replacement)
		m, ok := got.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, replacement, m["output"])
		assert.Equal(t, "stays", m["meta"])
		// original not mutated
		assert.Equal(t, "original output", details["output"])
	})

	t.Run("map with no content or output key returns copy", func(t *testing.T) {
		t.Parallel()
		details := map[string]any{
			"other": "value",
		}
		got := ReplaceText(details, replacement)
		m, ok := got.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "value", m["other"])
	})

	t.Run("no mutation on original map", func(t *testing.T) {
		t.Parallel()
		orig := map[string]any{"content": "before"}
		_ = ReplaceText(orig, replacement)
		assert.Equal(t, "before", orig["content"])
	})

	t.Run("extract after replace round-trips for content string", func(t *testing.T) {
		t.Parallel()
		details := map[string]any{"content": "original"}
		replaced := ReplaceText(details, replacement)
		text, ok := ExtractText(replaced)
		require.True(t, ok)
		assert.Equal(t, replacement, text)
	})

	t.Run("extract after replace round-trips for block array", func(t *testing.T) {
		t.Parallel()
		details := map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "old"},
			},
		}
		replaced := ReplaceText(details, replacement)
		text, ok := ExtractText(replaced)
		require.True(t, ok)
		assert.Equal(t, replacement, text)
	})
}
