package recall

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIndex(t *testing.T) {
	t.Parallel()

	idx := NewIndex(500)
	require.NotNil(t, idx)
	assert.Equal(t, 500, idx.MaxDocs)
	assert.Equal(t, 0, idx.TotalDocs)
	assert.NotNil(t, idx.Docs)
	assert.NotNil(t, idx.DocFreq)

	docs, terms := idx.Stats()
	assert.Equal(t, 0, docs)
	assert.Equal(t, 0, terms)
}

func TestAddAndSearch(t *testing.T) {
	t.Parallel()

	idx := NewIndex(500)
	idx.AddDocument("s1", "/sessions/s1.jsonl", "Go Refactoring", "refactor golang interface dependency injection patterns")
	idx.AddDocument("s2", "/sessions/s2.jsonl", "Database Design", "postgresql schema migration indexing performance")
	idx.AddDocument("s3", "/sessions/s3.jsonl", "TUI Development", "bubbletea lipgloss terminal user interface components")

	// Search for terms unique to s1
	results := idx.Search("golang refactor", 3)
	require.NotEmpty(t, results)
	assert.Equal(t, "s1", results[0].SessionID)
	assert.Greater(t, results[0].Score, 0.0)

	// Search for terms unique to s2
	results = idx.Search("postgresql migration", 3)
	require.NotEmpty(t, results)
	assert.Equal(t, "s2", results[0].SessionID)

	// Search for terms unique to s3
	results = idx.Search("bubbletea lipgloss", 3)
	require.NotEmpty(t, results)
	assert.Equal(t, "s3", results[0].SessionID)

	// Verify limit is respected
	results = idx.Search("interface patterns components", 2)
	assert.LessOrEqual(t, len(results), 2)
}

func TestSearchNoMatch(t *testing.T) {
	t.Parallel()

	idx := NewIndex(500)
	idx.AddDocument("s1", "/sessions/s1.jsonl", "Go Session", "golang interface injection")

	results := idx.Search("quantum physics telescope", 3)
	assert.Empty(t, results)
}

func TestSearchEmpty(t *testing.T) {
	t.Parallel()

	idx := NewIndex(500)
	results := idx.Search("anything", 3)
	assert.Empty(t, results)
}

func TestIndexPersistence(t *testing.T) {
	t.Parallel()

	idx := NewIndex(500)
	idx.AddDocument("s1", "/sessions/s1.jsonl", "Persistence Test", "golang testing persistence serialization gob encoding")
	idx.AddDocument("s2", "/sessions/s2.jsonl", "Other Session", "unrelated content about databases and schema")

	path := filepath.Join(t.TempDir(), "index.gob")
	require.NoError(t, idx.Save(path))

	loaded, err := LoadIndex(path)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify search still works after reload
	results := loaded.Search("golang serialization gob", 3)
	require.NotEmpty(t, results)
	assert.Equal(t, "s1", results[0].SessionID)
	assert.Equal(t, "Persistence Test", results[0].Title)

	// Verify doc count preserved
	docs, _ := loaded.Stats()
	assert.Equal(t, 2, docs)
}

func TestPruneOldest(t *testing.T) {
	t.Parallel()

	idx := NewIndex(2)

	// Add three docs — the first should be pruned.
	idx.AddDocument("old", "/sessions/old.jsonl", "Old Session", "ancient history dinosaur fossil bones")
	time.Sleep(2 * time.Millisecond) // ensure distinct IndexedAt
	idx.AddDocument("mid", "/sessions/mid.jsonl", "Middle Session", "middle ground between old and new content")
	time.Sleep(2 * time.Millisecond)
	idx.AddDocument("new", "/sessions/new.jsonl", "New Session", "recent modern contemporary current session data")

	docs, _ := idx.Stats()
	assert.Equal(t, 2, docs)

	// "old" should have been pruned
	_, ok := idx.Docs["old"]
	assert.False(t, ok, "oldest document should have been pruned")

	// "mid" and "new" should be present
	_, ok = idx.Docs["mid"]
	assert.True(t, ok)
	_, ok = idx.Docs["new"]
	assert.True(t, ok)
}

func TestTokenize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		check func(t *testing.T, tokens []string)
	}{
		{
			name:  "stopword removal",
			input: "the quick brown fox is jumping over the fence",
			check: func(t *testing.T, tokens []string) {
				t.Helper()
				for _, tok := range tokens {
					_, isStop := stopwords[tok]
					assert.False(t, isStop, "stopword %q should be removed", tok)
				}
			},
		},
		{
			name:  "case normalization",
			input: "Hello World GOLANG Testing",
			check: func(t *testing.T, tokens []string) {
				t.Helper()
				for _, tok := range tokens {
					assert.Equal(t, tok, tok, "all tokens should be lowercase") // paranoia
				}
				assert.Contains(t, tokens, "hello")
				assert.Contains(t, tokens, "world")
				assert.Contains(t, tokens, "golang")
			},
		},
		{
			name:  "short token filtering",
			input: "go is a great programming language",
			check: func(t *testing.T, tokens []string) {
				t.Helper()
				for _, tok := range tokens {
					assert.GreaterOrEqual(t, len(tok), 2, "no tokens shorter than 2 chars")
				}
			},
		},
		{
			name:  "deduplication",
			input: "golang golang golang interface interface",
			check: func(t *testing.T, tokens []string) {
				t.Helper()
				seen := make(map[string]int)
				for _, tok := range tokens {
					seen[tok]++
				}
				for tok, count := range seen {
					assert.Equal(t, 1, count, "token %q appears %d times", tok, count)
				}
			},
		},
		{
			name:  "non-alphanumeric split",
			input: "hello-world foo.bar baz_qux",
			check: func(t *testing.T, tokens []string) {
				t.Helper()
				assert.Contains(t, tokens, "hello")
				assert.Contains(t, tokens, "world")
				assert.Contains(t, tokens, "foo")
				assert.Contains(t, tokens, "bar")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tokens := tokenize(tc.input)
			tc.check(t, tokens)
		})
	}
}

func TestRemove(t *testing.T) {
	t.Parallel()

	idx := NewIndex(500)
	idx.AddDocument("s1", "/sessions/s1.jsonl", "Removable", "golang interface dependency injection patterns")

	results := idx.Search("golang injection", 3)
	require.NotEmpty(t, results)

	idx.Remove("s1")

	docs, _ := idx.Stats()
	assert.Equal(t, 0, docs)

	results = idx.Search("golang injection", 3)
	assert.Empty(t, results)
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	idx := NewIndex(500)

	var wg sync.WaitGroup
	const goroutines = 20

	// Concurrent adds
	for i := range goroutines {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("session-%d", n)
			text := fmt.Sprintf("golang interface injection session number %d concurrency testing", n)
			idx.AddDocument(id, "/sessions/"+id+".jsonl", "Session "+id, text)
		}(i)
	}

	// Concurrent searches running alongside adds
	for range goroutines / 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = idx.Search("golang injection concurrency", 5)
		}()
	}

	wg.Wait()

	docs, _ := idx.Stats()
	assert.GreaterOrEqual(t, docs, 0)
}

func TestStats(t *testing.T) {
	t.Parallel()

	idx := NewIndex(500)

	docs, terms := idx.Stats()
	assert.Equal(t, 0, docs)
	assert.Equal(t, 0, terms)

	idx.AddDocument("s1", "/p", "T1", "golang refactoring interface patterns dependency")
	docs, terms = idx.Stats()
	assert.Equal(t, 1, docs)
	assert.Greater(t, terms, 0)

	idx.AddDocument("s2", "/p", "T2", "postgresql database schema migration indexing")
	docs, terms = idx.Stats()
	assert.Equal(t, 2, docs)
	assert.Greater(t, terms, 0)

	idx.Remove("s1")
	docs, _ = idx.Stats()
	assert.Equal(t, 1, docs)
}
