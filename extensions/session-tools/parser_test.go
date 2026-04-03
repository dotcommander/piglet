package sessiontools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSessionFile creates a temporary JSONL session file from the given lines.
func writeSessionFile(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
	require.NoError(t, err)
	return path
}

// jsonLine builds a JSONL entry string from role, type, and a pre-encoded content value.
func jsonLine(role, typ, content string) string {
	return fmt.Sprintf(`{"type":%q,"role":%q,"content":%s}`, typ, role, content)
}

// TestExtractContent exercises all branches of extractContent directly.
func TestExtractContent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "string content",
			raw:  json.RawMessage(`"hello"`),
			want: "hello",
		},
		{
			name: "object with text field",
			raw:  json.RawMessage(`{"text":"world"}`),
			want: "world",
		},
		{
			name: "object without text field",
			raw:  json.RawMessage(`{"other":"value"}`),
			want: "",
		},
		{
			name: "array of text blocks",
			raw:  json.RawMessage(`[{"type":"text","text":"block one"},{"type":"text","text":"block two"}]`),
			want: "block one\nblock two",
		},
		{
			name: "array with empty text blocks skipped",
			raw:  json.RawMessage(`[{"type":"image","text":""},{"type":"text","text":"only this"}]`),
			want: "only this",
		},
		{
			name: "nil RawMessage",
			raw:  nil,
			want: "",
		},
		{
			name: "empty RawMessage",
			raw:  json.RawMessage{},
			want: "",
		},
		{
			name: "malformed JSON string",
			raw:  json.RawMessage(`"unterminated`),
			want: "",
		},
		{
			name: "malformed JSON object",
			raw:  json.RawMessage(`{bad json}`),
			want: "",
		},
		{
			name: "malformed JSON array",
			raw:  json.RawMessage(`[{bad}]`),
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractContent(tc.raw)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestQuerySession_NormalMatch checks that matching lines are returned with role prefix.
func TestQuerySession_NormalMatch(t *testing.T) {
	t.Parallel()

	path := writeSessionFile(t,
		jsonLine("user", "message", `"needle in a haystack"`),
		jsonLine("assistant", "message", `"no match here"`),
		jsonLine("user", "message", `"another needle found"`),
	)

	out, err := QuerySession(path, "needle", 10*1024*1024)
	require.NoError(t, err)

	assert.Contains(t, out, "[user]")
	assert.Contains(t, out, "needle in a haystack")
	assert.Contains(t, out, "another needle found")
	assert.NotContains(t, out, "no match here")
}

// TestQuerySession_NoMatches verifies the "No matches" message when nothing matches.
func TestQuerySession_NoMatches(t *testing.T) {
	t.Parallel()

	path := writeSessionFile(t,
		jsonLine("user", "message", `"hello world"`),
	)

	out, err := QuerySession(path, "xyznotpresent", 10*1024*1024)
	require.NoError(t, err)
	assert.Contains(t, out, "No matches for")
}

// TestQuerySession_FileNotFound asserts an error when the path does not exist.
func TestQuerySession_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := QuerySession("/nonexistent/path/session.jsonl", "query", 10*1024*1024)
	require.Error(t, err)
}

// TestQuerySession_FileTooLarge asserts an error when the file exceeds maxSize.
func TestQuerySession_FileTooLarge(t *testing.T) {
	t.Parallel()

	path := writeSessionFile(t,
		jsonLine("user", "message", `"some content"`),
	)

	_, err := QuerySession(path, "some", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

// TestQuerySession_MalformedJSONL checks that invalid lines are skipped and valid matches returned.
func TestQuerySession_MalformedJSONL(t *testing.T) {
	t.Parallel()

	path := writeSessionFile(t,
		`this is not json at all`,
		jsonLine("user", "message", `"valid and matching"`),
		`{broken json`,
		jsonLine("assistant", "message", `"also valid and matching"`),
	)

	out, err := QuerySession(path, "matching", 10*1024*1024)
	require.NoError(t, err)

	assert.Contains(t, out, "valid and matching")
	assert.Contains(t, out, "also valid and matching")
}

// TestQuerySession_TruncateAt50Matches verifies that results are capped at 50 with a truncation notice.
func TestQuerySession_TruncateAt50Matches(t *testing.T) {
	t.Parallel()

	lines := make([]string, 60)
	for i := range lines {
		lines[i] = jsonLine("user", "message", fmt.Sprintf(`"match number %d"`, i))
	}
	path := writeSessionFile(t, lines...)

	out, err := QuerySession(path, "match", 10*1024*1024)
	require.NoError(t, err)

	assert.Contains(t, out, "showing first 50")

	// Count occurrences of "[user]" — should be exactly 50.
	count := strings.Count(out, "[user]")
	assert.Equal(t, 50, count)
}

// TestQuerySession_ContentTruncation verifies that content longer than 500 runes is truncated with "...".
func TestQuerySession_ContentTruncation(t *testing.T) {
	t.Parallel()

	longContent := strings.Repeat("x", 600)
	path := writeSessionFile(t,
		jsonLine("user", "message", fmt.Sprintf(`%q`, longContent)),
	)

	out, err := QuerySession(path, "x", 10*1024*1024)
	require.NoError(t, err)
	assert.Contains(t, out, "...")

	// The content portion must not exceed 503 characters (500 runes + "...").
	assert.NotContains(t, out, strings.Repeat("x", 501))
}

// TestQuerySession_RoleFallbackToType checks that an empty role falls back to the type field.
func TestQuerySession_RoleFallbackToType(t *testing.T) {
	t.Parallel()

	// Build a line with empty role and type "system".
	line := `{"type":"system","role":"","content":"system prompt content"}`
	path := writeSessionFile(t, line)

	out, err := QuerySession(path, "system prompt", 10*1024*1024)
	require.NoError(t, err)
	assert.Contains(t, out, "[system]")
}

// TestQuerySession_LargeLineWithinBuffer ensures lines up to ~900K succeed within a 2MB maxSize.
func TestQuerySession_LargeLineWithinBuffer(t *testing.T) {
	t.Parallel()

	bigContent := strings.Repeat("a", 900*1024)
	line := jsonLine("user", "message", fmt.Sprintf(`%q`, bigContent))
	path := writeSessionFile(t, line)

	_, err := QuerySession(path, "aaa", 2*1024*1024)
	require.NoError(t, err)
}
