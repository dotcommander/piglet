package memory_test

import (
	"testing"

	"github.com/dotcommander/piglet/extensions/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelated(t *testing.T) {
	t.Parallel()

	t.Run("linear chain depth 1", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		s, err := memory.NewStore(cwd)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		// a → b → c
		require.NoError(t, s.Set("a", "1", ""))
		require.NoError(t, s.Set("b", "2", ""))
		require.NoError(t, s.Set("c", "3", ""))
		require.NoError(t, s.Relate("a", "b"))
		require.NoError(t, s.Relate("b", "c"))

		facts := memory.Related(s, "a", 1)
		assert.Len(t, facts, 1) // only b at depth 1
		assert.Equal(t, "b", facts[0].Key)
	})

	t.Run("linear chain depth 2", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		s, err := memory.NewStore(cwd)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		// a → b → c
		require.NoError(t, s.Set("a", "1", ""))
		require.NoError(t, s.Set("b", "2", ""))
		require.NoError(t, s.Set("c", "3", ""))
		require.NoError(t, s.Relate("a", "b"))
		require.NoError(t, s.Relate("b", "c"))

		facts := memory.Related(s, "a", 2)
		assert.Len(t, facts, 2) // b and c
		keys := []string{facts[0].Key, facts[1].Key}
		assert.Contains(t, keys, "b")
		assert.Contains(t, keys, "c")
	})

	t.Run("nonexistent key returns empty", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		s, err := memory.NewStore(cwd)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		facts := memory.Related(s, "nope", 3)
		assert.Empty(t, facts)
	})

	t.Run("no relations returns empty", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		s, err := memory.NewStore(cwd)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		require.NoError(t, s.Set("lonely", "alone", ""))
		facts := memory.Related(s, "lonely", 3)
		assert.Empty(t, facts)
	})

	t.Run("cycle does not infinite loop", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		s, err := memory.NewStore(cwd)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		require.NoError(t, s.Set("a", "1", ""))
		require.NoError(t, s.Set("b", "2", ""))
		require.NoError(t, s.Set("c", "3", ""))
		require.NoError(t, s.Relate("a", "b"))
		require.NoError(t, s.Relate("b", "c"))
		require.NoError(t, s.Relate("c", "a")) // cycle

		facts := memory.Related(s, "a", 10)
		assert.Len(t, facts, 2) // b and c, no duplicates
	})

	t.Run("results sorted by key", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		s, err := memory.NewStore(cwd)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		require.NoError(t, s.Set("hub", "center", ""))
		require.NoError(t, s.Set("zebra", "z", ""))
		require.NoError(t, s.Set("apple", "a", ""))
		require.NoError(t, s.Set("mango", "m", ""))
		require.NoError(t, s.Relate("hub", "zebra"))
		require.NoError(t, s.Relate("hub", "apple"))
		require.NoError(t, s.Relate("hub", "mango"))

		facts := memory.Related(s, "hub", 1)
		require.Len(t, facts, 3)
		assert.Equal(t, "apple", facts[0].Key)
		assert.Equal(t, "mango", facts[1].Key)
		assert.Equal(t, "zebra", facts[2].Key)
	})
}
