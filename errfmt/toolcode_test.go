package errfmt_test

import (
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/errfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolErr_Shape(t *testing.T) {
	t.Parallel()

	result := errfmt.ToolErr(errfmt.ToolErrFileStale, "stale", "reread")

	require.NotNil(t, result)
	require.Len(t, result.Content, 1)

	tc, ok := result.Content[0].(core.TextContent)
	require.True(t, ok, "expected TextContent block")
	assert.Equal(t, "[error:FILE_STALE] stale\nhint: reread", tc.Text)
}

func TestToolErr_NoHint(t *testing.T) {
	t.Parallel()

	result := errfmt.ToolErr("X", "msg", "")

	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(core.TextContent)
	require.True(t, ok)
	assert.Equal(t, "[error:X] msg", tc.Text)
}

func TestParseToolError_Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code    errfmt.ToolErrorCode
		summary string
		hint    string
	}{
		{errfmt.ToolErrFileStale, "file modified since last read", "re-read before writing"},
		{errfmt.ToolErrInvalidArgs, "path is required", "provide an absolute file path"},
		{errfmt.ToolErrNotUnique, "old_text matched 3 locations", "add surrounding context"},
		{errfmt.ToolErrExitNonzero, "exit code 2", ""},
		{"CUSTOM_CODE", "custom error message", ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()

			result := errfmt.ToolErr(tc.code, tc.summary, tc.hint)
			require.Len(t, result.Content, 1)
			text := result.Content[0].(core.TextContent).Text

			parsed := errfmt.ParseToolError(text)
			require.NotNil(t, parsed)
			assert.Equal(t, tc.code, parsed.Code)
			assert.Equal(t, tc.summary, parsed.Summary)
			assert.Equal(t, tc.hint, parsed.Hint)
			assert.Empty(t, parsed.Body)
		})
	}
}

func TestParseToolError_NonCoded(t *testing.T) {
	t.Parallel()

	assert.Nil(t, errfmt.ParseToolError("plain text"))
	assert.Nil(t, errfmt.ParseToolError("error: something went wrong"))
	assert.Nil(t, errfmt.ParseToolError(""))
}

func TestParseToolError_WithBody(t *testing.T) {
	t.Parallel()

	// Simulate toolBashErr output: header + hint + blank line + captured output.
	result := errfmt.ToolErr(errfmt.ToolErrExitNonzero, "exit code 2", "inspect stderr")
	base := result.Content[0].(core.TextContent).Text
	withBody := base + "\n\nSTDERR:\nfoo: not found"

	parsed := errfmt.ParseToolError(withBody)
	require.NotNil(t, parsed)
	assert.Equal(t, errfmt.ToolErrExitNonzero, parsed.Code)
	assert.Equal(t, "exit code 2", parsed.Summary)
	assert.Equal(t, "inspect stderr", parsed.Hint)
	assert.Equal(t, "STDERR:\nfoo: not found", parsed.Body)
}
