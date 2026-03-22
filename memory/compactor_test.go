package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dotcommander/piglet/core"
)

func TestCompactFnMinMessages(t *testing.T) {
	t.Parallel()
	store := testStore(t)
	fn := CompactFn(store, nil)

	// With fewer than keepRecent+1 messages, should return unchanged
	msgs := []core.Message{
		&core.UserMessage{Content: "hello"},
		&core.AssistantMessage{Content: []core.AssistantContent{core.TextContent{Text: "hi"}}},
	}

	result, err := fn(context.Background(), msgs)
	require.NoError(t, err)
	assert.Equal(t, msgs, result)
}

func TestCompactFnWithFacts(t *testing.T) {
	t.Parallel()
	store := testStore(t)

	// Pre-populate some context facts
	_ = store.Set("ctx:file:/src/main.go", "read, 50 lines", contextCategory)
	_ = store.Set("ctx:edit:/src/main.go", "added handler", contextCategory)
	_ = store.Set("ctx:error:1", "go build: undefined Foo", contextCategory)

	fn := CompactFn(store, nil) // no LLM provider

	// Build enough messages to trigger compaction
	msgs := make([]core.Message, 0, 10)
	msgs = append(msgs, &core.UserMessage{Content: "initial task"})
	for range 9 {
		msgs = append(msgs, &core.AssistantMessage{
			Content: []core.AssistantContent{core.TextContent{Text: "response"}},
		})
	}

	result, err := fn(context.Background(), msgs)
	require.NoError(t, err)

	// Should be compacted: first + summary + last 6 = 8
	assert.Equal(t, keepRecent+2, len(result))

	// Summary message should reference memory
	if am, ok := result[1].(*core.AssistantMessage); ok {
		text := am.Content[0].(core.TextContent).Text
		assert.Contains(t, text, "Context compacted")
		assert.Contains(t, text, "memory_list")
		assert.Contains(t, text, "main.go")
	}

	// ctx:summary should be stored
	summary, ok := store.Get("ctx:summary")
	assert.True(t, ok)
	assert.Contains(t, summary.Value, "main.go")
}

func TestBuildFactSummary(t *testing.T) {
	t.Parallel()

	facts := []Fact{
		{Key: "ctx:file:/src/a.go", Value: "read, 100 lines"},
		{Key: "ctx:file:/src/b.go", Value: "read, 50 lines"},
		{Key: "ctx:edit:/src/a.go", Value: "added New() constructor"},
		{Key: "ctx:error:1", Value: "build failed: undefined X"},
		{Key: "ctx:cmd:2", Value: "go test ./... — all passed"},
	}

	summary := buildFactSummary(facts)
	assert.Contains(t, summary, "/src/a.go")
	assert.Contains(t, summary, "/src/b.go")
	assert.Contains(t, summary, "added New() constructor")
	assert.Contains(t, summary, "undefined X")
	assert.Contains(t, summary, "go test")
}

func TestFirstLine(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello", firstLine("hello\nworld"))
	assert.Equal(t, "short", firstLine("short"))
}
