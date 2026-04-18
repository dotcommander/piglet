package compact

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mapStorer is a minimal in-memory Storer for tests.
// Avoids importing extensions/memory to prevent import cycles.
type mapStorer struct {
	data map[string]Fact
}

func newMapStorer() *mapStorer {
	return &mapStorer{data: make(map[string]Fact)}
}

func (m *mapStorer) List(category string) []Fact {
	var out []Fact
	for _, f := range m.data {
		if category == "" || f.Category == category {
			out = append(out, f)
		}
	}
	return out
}

func (m *mapStorer) Set(key, value, category string) error {
	m.data[key] = Fact{
		Key:       key,
		Value:     value,
		Category:  category,
		UpdatedAt: time.Now(),
	}
	return nil
}

func (m *mapStorer) Get(key string) (Fact, bool) {
	f, ok := m.data[key]
	return f, ok
}

func (m *mapStorer) Clear() error {
	m.data = make(map[string]Fact)
	return nil
}

func TestCompactNoFacts(t *testing.T) {
	t.Parallel()
	store := newMapStorer()

	result := Compact(store)
	assert.Equal(t, "", result.Summary)
	assert.Equal(t, 0, result.FactCount)
}

func TestCompactWithFacts(t *testing.T) {
	t.Parallel()
	store := newMapStorer()

	_ = store.Set("ctx:file:/src/main.go", "read, 50 lines", contextCategory)
	_ = store.Set("ctx:edit:/src/main.go", "added handler", contextCategory)
	_ = store.Set("ctx:error:1", "go build: undefined Foo", contextCategory)

	result := Compact(store)
	assert.Equal(t, 3, result.FactCount)
	assert.Contains(t, result.Summary, "/src/main.go")
	assert.Contains(t, result.Summary, "added handler")
	assert.Contains(t, result.Summary, "undefined Foo")
}

func TestWriteSummary(t *testing.T) {
	t.Parallel()
	store := newMapStorer()

	WriteSummary(store, "session summary text")

	fact, ok := store.Get("ctx:summary")
	assert.True(t, ok)
	assert.Equal(t, "session summary text", fact.Value)
}

func TestWriteSummaryEmpty(t *testing.T) {
	t.Parallel()
	store := newMapStorer()

	WriteSummary(store, "")

	_, ok := store.Get("ctx:summary")
	assert.False(t, ok)
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
