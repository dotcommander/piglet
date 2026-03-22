package memory

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractorFileRead(t *testing.T) {
	t.Parallel()
	store := testStore(t)
	ext := NewExtractor(store)

	data := mustMarshal(t, TurnData{
		ToolResults: []json.RawMessage{
			mustMarshal(t, toolResult{
				ToolName: "Read",
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{{Type: "text", Text: "/Users/test/main.go\n1→ package main\n2→ func main() {}"}},
			}),
		},
	})

	require.NoError(t, ext.Extract(data))

	fact, ok := store.Get("ctx:file:/Users/test/main.go")
	assert.True(t, ok)
	assert.Contains(t, fact.Value, "package main")
	assert.Equal(t, contextCategory, fact.Category)
}

func TestExtractorBashError(t *testing.T) {
	t.Parallel()
	store := testStore(t)
	ext := NewExtractor(store)

	data := mustMarshal(t, TurnData{
		ToolResults: []json.RawMessage{
			mustMarshal(t, toolResult{
				ToolName: "Bash",
				IsError:  true,
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{{Type: "text", Text: "go build: undefined Foo"}},
			}),
		},
	})

	require.NoError(t, ext.Extract(data))

	facts := store.List(contextCategory)
	require.Len(t, facts, 1)
	assert.Contains(t, facts[0].Key, "ctx:error:")
	assert.Contains(t, facts[0].Value, "undefined Foo")
}

func TestExtractorPrune(t *testing.T) {
	t.Parallel()
	store := testStore(t)
	ext := NewExtractor(store)

	// Fill to over the cap
	for range maxContextFacts + 5 {
		data := mustMarshal(t, TurnData{
			ToolResults: []json.RawMessage{
				mustMarshal(t, toolResult{
					ToolName: "Bash",
					Content: []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					}{{Type: "text", Text: "ok"}},
				}),
			},
		})
		_ = ext.Extract(data)
	}

	facts := store.List(contextCategory)
	assert.LessOrEqual(t, len(facts), maxContextFacts)
}

func TestExtractFilePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"/Users/test/main.go some text", "/Users/test/main.go"},
		{"/absolute/path", "/absolute/path"},
		{"some text with src/pkg/file.go in it", "src/pkg/file.go"},
		{"no path here", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, extractFilePath(tt.input), "input: %s", tt.input)
	}
}

func TestTruncRunes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "abc...", truncRunes("abcdef", 3))
	assert.Equal(t, "ab", truncRunes("ab", 3))
	assert.Equal(t, "日本語...", truncRunes("日本語テスト", 3))
}

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "test-project"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Clear() })
	return s
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
