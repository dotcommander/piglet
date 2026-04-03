package memory

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/facts.jsonl"
	s := &Store{
		path: path,
		data: make(map[string]Fact),
	}
	return s
}

func TestFilterFacts_Empty(t *testing.T) {
	t.Parallel()
	got := filterFacts(nil, "ctx:edit:")
	assert.Empty(t, got)
}

func TestFilterFacts_NoMatch(t *testing.T) {
	t.Parallel()
	facts := []Fact{
		{Key: "ctx:file:main.go", Value: "read"},
		{Key: "ctx:cmd:build", Value: "ran"},
	}
	got := filterFacts(facts, "ctx:edit:")
	assert.Empty(t, got)
}

func TestFilterFacts_AllMatch(t *testing.T) {
	t.Parallel()
	facts := []Fact{
		{Key: "ctx:edit:a.go", Value: "edited a"},
		{Key: "ctx:edit:b.go", Value: "edited b"},
	}
	got := filterFacts(facts, "ctx:edit:")
	assert.Len(t, got, 2)
}

func TestFilterFacts_SortedByUpdatedAtDesc(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	facts := []Fact{
		{Key: "ctx:edit:a.go", UpdatedAt: base.Add(1 * time.Hour)},
		{Key: "ctx:edit:b.go", UpdatedAt: base.Add(3 * time.Hour)},
		{Key: "ctx:edit:c.go", UpdatedAt: base.Add(2 * time.Hour)},
	}
	got := filterFacts(facts, "ctx:edit:")
	require.Len(t, got, 3)
	// Most recently updated first
	assert.Equal(t, "ctx:edit:b.go", got[0].Key)
	assert.Equal(t, "ctx:edit:c.go", got[1].Key)
	assert.Equal(t, "ctx:edit:a.go", got[2].Key)
}

func TestFilterFacts_MixedPrefixes(t *testing.T) {
	t.Parallel()
	facts := []Fact{
		{Key: "ctx:edit:a.go", UpdatedAt: time.Now()},
		{Key: "ctx:file:a.go", UpdatedAt: time.Now()},
		{Key: "ctx:edit:b.go", UpdatedAt: time.Now()},
		{Key: "other:key", UpdatedAt: time.Now()},
	}
	editFacts := filterFacts(facts, "ctx:edit:")
	assert.Len(t, editFacts, 2)
	fileFacts := filterFacts(facts, "ctx:file:")
	assert.Len(t, fileFacts, 1)
}

func TestTotalTokens_Empty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, totalTokens(nil))
	assert.Equal(t, 0, totalTokens([]criticalContext{}))
}

func TestTotalTokens_SingleItem(t *testing.T) {
	t.Parallel()
	items := []criticalContext{
		{category: "recent edits", content: strings.Repeat("x", 40)},
	}
	// 40 chars / 4 = 10 tokens
	assert.Equal(t, 10, totalTokens(items))
}

func TestTotalTokens_MultipleItems(t *testing.T) {
	t.Parallel()
	items := []criticalContext{
		{content: strings.Repeat("a", 40)},  // 10 tokens
		{content: strings.Repeat("b", 80)},  // 20 tokens
		{content: strings.Repeat("c", 120)}, // 30 tokens
	}
	assert.Equal(t, 60, totalTokens(items))
}

func TestBuildReinjectMessage_Empty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", buildReinjectMessage(nil))
	assert.Equal(t, "", buildReinjectMessage([]criticalContext{}))
}

func TestBuildReinjectMessage_SingleItem(t *testing.T) {
	t.Parallel()
	items := []criticalContext{
		{category: "recent edits", content: "ctx:edit:main.go: updated handler"},
	}
	got := buildReinjectMessage(items)
	assert.Contains(t, got, "[Post-compact context preservation]")
	assert.Contains(t, got, "recent edits")
	assert.Contains(t, got, "ctx:edit:main.go: updated handler")
}

func TestBuildReinjectMessage_MultipleItems(t *testing.T) {
	t.Parallel()
	items := []criticalContext{
		{category: "recent edits", content: "ctx:edit:a.go: added func"},
		{category: "active plan", content: "ctx:plan:1: implement feature"},
	}
	got := buildReinjectMessage(items)
	assert.Contains(t, got, "recent edits")
	assert.Contains(t, got, "active plan")
	assert.Contains(t, got, "ctx:edit:a.go")
	assert.Contains(t, got, "ctx:plan:1")
}

func TestGatherCriticalContext_EmptyStore(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	items := gatherCriticalContext(s)
	assert.Empty(t, items)
}

func TestGatherCriticalContext_WithEditFacts(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	base := time.Now().UTC()
	// Insert directly to avoid file I/O issues in parallel tests
	s.data["ctx:edit:a.go"] = Fact{Key: "ctx:edit:a.go", Value: "edited a", Category: contextCategory, UpdatedAt: base.Add(1 * time.Second)}
	s.data["ctx:edit:b.go"] = Fact{Key: "ctx:edit:b.go", Value: "edited b", Category: contextCategory, UpdatedAt: base.Add(2 * time.Second)}
	s.data["ctx:edit:c.go"] = Fact{Key: "ctx:edit:c.go", Value: "edited c", Category: contextCategory, UpdatedAt: base.Add(3 * time.Second)}

	items := gatherCriticalContext(s)
	// appendItems takes up to 3 recent edits
	assert.LessOrEqual(t, len(items), 3)
	assert.Greater(t, len(items), 0)
}

func TestGatherCriticalContext_RespectMaxItems(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	base := time.Now().UTC()
	// Add more facts than reinjectMaxItems across categories
	for i := range reinjectMaxItems + 2 {
		key := "ctx:edit:file" + string(rune('a'+i)) + ".go"
		s.data[key] = Fact{
			Key:       key,
			Value:     "edited",
			Category:  contextCategory,
			UpdatedAt: base.Add(time.Duration(i) * time.Second),
		}
	}

	items := gatherCriticalContext(s)
	assert.LessOrEqual(t, len(items), reinjectMaxItems)
}

func TestGatherCriticalContext_ContentTruncated(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Add a fact with content exceeding per-item char limit
	longValue := strings.Repeat("x", reinjectMaxPerItem*reinjectCharsPerTok+100)
	s.data["ctx:edit:big.go"] = Fact{
		Key:       "ctx:edit:big.go",
		Value:     longValue,
		Category:  contextCategory,
		UpdatedAt: time.Now().UTC(),
	}

	items := gatherCriticalContext(s)
	require.NotEmpty(t, items)
	// Content should be truncated with "..."
	assert.True(t, strings.HasSuffix(items[0].content, "..."))
}

func TestGatherCriticalContext_WithPlanFact(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	s.data["ctx:plan:1"] = Fact{
		Key:       "ctx:plan:1",
		Value:     "implement the feature",
		Category:  contextCategory,
		UpdatedAt: time.Now().UTC(),
	}

	items := gatherCriticalContext(s)
	require.NotEmpty(t, items)
	found := false
	for _, item := range items {
		if item.category == "active plan" {
			found = true
		}
	}
	assert.True(t, found, "expected active plan category in results")
}

func TestGatherCriticalContext_WithErrorFact(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	s.data["ctx:error:1"] = Fact{
		Key:       "ctx:error:1",
		Value:     "undefined: Foo",
		Category:  contextCategory,
		UpdatedAt: time.Now().UTC(),
	}

	items := gatherCriticalContext(s)
	require.NotEmpty(t, items)
	found := false
	for _, item := range items {
		if item.category == "recent errors" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestReinjectConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 50000, reinjectMaxTokens)
	assert.Equal(t, 5000, reinjectMaxPerItem)
	assert.Equal(t, 5, reinjectMaxItems)
	assert.Equal(t, 4, reinjectCharsPerTok)
}

// Ensure newTestStore helper doesn't accidentally write to real config.
func TestNewTestStore_UsesTemp(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	assert.NotEmpty(t, s.path)
	// Path should be in a temp directory, not the user's home
	home, _ := os.UserHomeDir()
	assert.False(t, strings.HasPrefix(s.path, home+"/"), "store path should not be in home dir")
}
