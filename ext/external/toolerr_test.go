package external

import (
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/stretchr/testify/require"
)

func TestEnsureCodedErrorPrefix_AddsPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     string
		hint     string
		input    string
		expected string
	}{
		{
			name:     "with hint",
			code:     "FILE_STALE",
			hint:     "reread",
			input:    "original text",
			expected: "[error:FILE_STALE] original text\nhint: reread",
		},
		{
			name:     "without hint",
			code:     "FILE_STALE",
			hint:     "",
			input:    "original text",
			expected: "[error:FILE_STALE] original text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			blocks := []core.ContentBlock{core.TextContent{Text: tt.input}}
			got := ensureCodedErrorPrefix(blocks, tt.code, tt.hint)
			require.Len(t, got, 1)
			tc, ok := got[0].(core.TextContent)
			require.True(t, ok)
			require.Equal(t, tt.expected, tc.Text)
		})
	}
}

func TestEnsureCodedErrorPrefix_Idempotent(t *testing.T) {
	t.Parallel()

	input := "[error:FILE_STALE] already formatted\nhint: do something"
	blocks := []core.ContentBlock{core.TextContent{Text: input}}
	got := ensureCodedErrorPrefix(blocks, "FILE_STALE", "reread")
	require.Len(t, got, 1)
	tc, ok := got[0].(core.TextContent)
	require.True(t, ok)
	require.Equal(t, input, tc.Text, "text already starting with [error: must be unchanged")
}

func TestEnsureCodedErrorPrefix_EmptyCode(t *testing.T) {
	t.Parallel()

	input := "some text"
	blocks := []core.ContentBlock{core.TextContent{Text: input}}
	got := ensureCodedErrorPrefix(blocks, "", "a hint")
	require.Len(t, got, 1)
	tc, ok := got[0].(core.TextContent)
	require.True(t, ok)
	require.Equal(t, input, tc.Text, "empty code must leave text unchanged")
}

func TestEnsureCodedErrorPrefix_NonTextBlock(t *testing.T) {
	t.Parallel()

	blocks := []core.ContentBlock{core.ImageContent{Data: "base64data", MimeType: "image/png"}}
	got := ensureCodedErrorPrefix(blocks, "FILE_STALE", "reread")
	require.Len(t, got, 1)
	_, isImage := got[0].(core.ImageContent)
	require.True(t, isImage, "non-text first block must be returned unchanged")
}
