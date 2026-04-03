package recall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeJSONL writes a session JSONL file to dir and returns its path.
func writeJSONL(t *testing.T, dir string, entries []map[string]any) string {
	t.Helper()
	path := filepath.Join(dir, "session.jsonl")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, e := range entries {
		require.NoError(t, enc.Encode(e))
	}
	return path
}

// userMsg builds a JSONL entry for a user message.
func userMsg(content string) map[string]any {
	return map[string]any{
		"type": "message",
		"data": map[string]any{"role": "user", "content": content},
	}
}

// assistantMsg builds a JSONL entry for an assistant text message.
func assistantMsg(content string) map[string]any {
	return map[string]any{
		"type": "message",
		"data": map[string]any{"role": "assistant", "content": content},
	}
}

// metaEntry builds a JSONL meta entry.
func metaEntry() map[string]any {
	return map[string]any{
		"type": "meta",
		"data": map[string]any{"version": 1, "cwd": "/tmp"},
	}
}

// toolResultMsg builds a JSONL entry with a tool_result content block.
func toolResultMsg() map[string]any {
	return map[string]any{
		"type": "message",
		"data": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "tool_result", "tool_use_id": "id1", "content": "file contents here"},
			},
		},
	}
}

func TestExtractSessionText(t *testing.T) {
	t.Parallel()

	path := writeJSONL(t, t.TempDir(), []map[string]any{
		metaEntry(),
		userMsg("How do I refactor Go interfaces?"),
		assistantMsg("Use dependency injection and define narrow interfaces."),
		userMsg("Can you show an example?"),
	})

	text, err := ExtractSessionText(path, 0)
	require.NoError(t, err)

	assert.Contains(t, text, "How do I refactor Go interfaces?")
	assert.Contains(t, text, "dependency injection")
	assert.Contains(t, text, "Can you show an example?")
	// meta entry must not appear
	assert.NotContains(t, text, "version")
}

func TestExtractSessionTextTruncation(t *testing.T) {
	t.Parallel()

	path := writeJSONL(t, t.TempDir(), []map[string]any{
		userMsg(strings.Repeat("golang ", 500)),
		assistantMsg(strings.Repeat("refactoring ", 500)),
	})

	text, err := ExtractSessionText(path, 100)
	require.NoError(t, err)
	assert.LessOrEqual(t, len([]rune(text)), 100)
}

func TestExtractSessionTextSkipsMeta(t *testing.T) {
	t.Parallel()

	path := writeJSONL(t, t.TempDir(), []map[string]any{
		metaEntry(),
		metaEntry(),
		metaEntry(),
		userMsg("only real content here"),
	})

	text, err := ExtractSessionText(path, 0)
	require.NoError(t, err)
	assert.Contains(t, text, "only real content here")
	assert.NotContains(t, text, "cwd")
	assert.NotContains(t, text, "version")
}

func TestExtractSessionTextSkipsToolResults(t *testing.T) {
	t.Parallel()

	path := writeJSONL(t, t.TempDir(), []map[string]any{
		userMsg("what is in the file?"),
		toolResultMsg(),
		assistantMsg("the file contains configuration data"),
	})

	text, err := ExtractSessionText(path, 0)
	require.NoError(t, err)
	assert.Contains(t, text, "what is in the file?")
	assert.Contains(t, text, "configuration data")
	// tool_result content block text should not appear
	assert.NotContains(t, text, "file contents here")
}

func TestExtractSessionTextMissingFile(t *testing.T) {
	t.Parallel()

	_, err := ExtractSessionText("/nonexistent/path/session.jsonl", 0)
	require.Error(t, err)
}
