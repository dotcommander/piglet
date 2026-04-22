package memory_test

import (
	"os"
	"testing"

	"github.com/dotcommander/piglet/extensions/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportance(t *testing.T) {
	t.Parallel()

	t.Run("defaults to zero", func(t *testing.T) {
		t.Parallel()
		s, err := memory.NewStore(t.TempDir())
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		require.NoError(t, s.Set("k", "v", ""))
		f, ok := s.Get("k")
		require.True(t, ok)
		assert.Equal(t, 0, f.Importance)
	})

	t.Run("SetImportance updates and persists", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		s, err := memory.NewStore(cwd)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		require.NoError(t, s.Set("critical", "val", "config"))
		require.NoError(t, s.SetImportance("critical", 10))

		f, ok := s.Get("critical")
		require.True(t, ok)
		assert.Equal(t, 10, f.Importance)

		// Verify persistence across reload.
		s2, err := memory.NewStore(cwd)
		require.NoError(t, err)
		f2, ok := s2.Get("critical")
		require.True(t, ok)
		assert.Equal(t, 10, f2.Importance)
	})

	t.Run("SetImportance on missing key is no-op", func(t *testing.T) {
		t.Parallel()
		s, err := memory.NewStore(t.TempDir())
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		require.NoError(t, s.SetImportance("ghost", 5))
		assert.Empty(t, s.List(""))
	})

	t.Run("omitted from JSON when zero", func(t *testing.T) {
		t.Parallel()
		s, err := memory.NewStore(t.TempDir())
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		require.NoError(t, s.Set("z", "val", ""))
		raw, err := os.ReadFile(s.Path())
		require.NoError(t, err)
		assert.NotContains(t, string(raw), `"importance"`)
	})

	t.Run("present in JSON when nonzero", func(t *testing.T) {
		t.Parallel()
		s, err := memory.NewStore(t.TempDir())
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Clear() })

		require.NoError(t, s.Set("p", "val", ""))
		require.NoError(t, s.SetImportance("p", 7))
		raw, err := os.ReadFile(s.Path())
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"importance":7`)
	})
}
