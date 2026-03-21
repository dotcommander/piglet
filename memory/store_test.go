package memory_test

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	s, err := memory.NewStore(cwd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Clear() })

	// Set 3 facts: two with category "config", one uncategorized.
	require.NoError(t, s.Set("alpha", "value-a", "config"))
	require.NoError(t, s.Set("beta", "value-b", "config"))
	require.NoError(t, s.Set("gamma", "value-g", ""))

	t.Run("Get each fact", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			key      string
			wantVal  string
			wantCat  string
		}{
			{"alpha", "value-a", "config"},
			{"beta", "value-b", "config"},
			{"gamma", "value-g", ""},
		}
		for _, tc := range cases {
			t.Run(tc.key, func(t *testing.T) {
				t.Parallel()
				f, ok := s.Get(tc.key)
				require.True(t, ok, "expected key %q to exist", tc.key)
				assert.Equal(t, tc.wantVal, f.Value)
				assert.Equal(t, tc.wantCat, f.Category)
			})
		}
	})

	t.Run("Get non-existent key", func(t *testing.T) {
		t.Parallel()
		_, ok := s.Get("no-such-key")
		assert.False(t, ok)
	})

	t.Run("List all returns 3 items sorted by key", func(t *testing.T) {
		t.Parallel()
		facts := s.List("")
		require.Len(t, facts, 3)
		assert.Equal(t, "alpha", facts[0].Key)
		assert.Equal(t, "beta", facts[1].Key)
		assert.Equal(t, "gamma", facts[2].Key)
	})

	t.Run("List by category config returns 2 items", func(t *testing.T) {
		t.Parallel()
		facts := s.List("config")
		require.Len(t, facts, 2)
		for _, f := range facts {
			assert.Equal(t, "config", f.Category)
		}
	})

	t.Run("Delete one then verify gone", func(t *testing.T) {
		t.Parallel()

		// Use a fresh store to avoid racing with other subtests.
		cwd2 := t.TempDir()
		s2, err := memory.NewStore(cwd2)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s2.Clear() })

		require.NoError(t, s2.Set("x", "1", ""))
		require.NoError(t, s2.Set("y", "2", ""))

		require.NoError(t, s2.Delete("x"))

		_, ok := s2.Get("x")
		assert.False(t, ok, "deleted key should not be found")

		_, ok = s2.Get("y")
		assert.True(t, ok, "non-deleted key should still exist")
	})

	t.Run("Clear leaves store empty", func(t *testing.T) {
		t.Parallel()

		cwd2 := t.TempDir()
		s2, err := memory.NewStore(cwd2)
		require.NoError(t, err)

		require.NoError(t, s2.Set("k", "v", ""))
		require.NoError(t, s2.Clear())

		facts := s2.List("")
		assert.Empty(t, facts)
	})
}

func TestStorePersistence(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()

	s1, err := memory.NewStore(cwd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s1.Clear() })

	require.NoError(t, s1.Set("persist-key", "persist-value", ""))

	// Backing file must exist at the reported path.
	_, statErr := os.Stat(s1.Path())
	require.NoError(t, statErr, "backing file should exist after Set")

	// A new store opening the same cwd should reload the fact.
	s2, err := memory.NewStore(cwd)
	require.NoError(t, err)

	f, ok := s2.Get("persist-key")
	require.True(t, ok, "fact should survive store reload")
	assert.Equal(t, "persist-value", f.Value)
}

func TestStoreOverwrite(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	s, err := memory.NewStore(cwd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Clear() })

	require.NoError(t, s.Set("key", "first", ""))
	f1, ok := s.Get("key")
	require.True(t, ok)
	createdAt := f1.CreatedAt

	require.NoError(t, s.Set("key", "second", ""))
	f2, ok := s.Get("key")
	require.True(t, ok)

	assert.Equal(t, "second", f2.Value, "value should be updated")
	assert.Equal(t, createdAt.UTC(), f2.CreatedAt.UTC(), "CreatedAt should be preserved on overwrite")
	assert.True(t, f2.UpdatedAt.After(f1.UpdatedAt) || f2.UpdatedAt.Equal(f1.UpdatedAt),
		"UpdatedAt should be >= first UpdatedAt after overwrite")
}

func TestStoreConcurrent(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	s, err := memory.NewStore(cwd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Clear() })

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)

	errs := make([]error, workers)
	for i := range workers {
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("worker-%d", i)
			errs[i] = s.Set(key, fmt.Sprintf("val-%d", i), "")
		}()
	}

	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "worker %d Set failed", i)
	}

	facts := s.List("")
	assert.Len(t, facts, workers, "all concurrent writes should be present")
}

func TestStoreEmptyCwd(t *testing.T) {
	t.Parallel()

	s, err := memory.NewStore("")
	require.NoError(t, err, "empty cwd should produce a valid store")
	t.Cleanup(func() { _ = s.Clear() })

	assert.NotEmpty(t, s.Path())
}

func TestRegister(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	memory.Register(app)

	tools := app.Tools()
	assert.Contains(t, tools, "memory_set")
	assert.Contains(t, tools, "memory_get")
	assert.Contains(t, tools, "memory_list")

	commands := app.Commands()
	_, ok := commands["memory"]
	assert.True(t, ok, "expected /memory command to be registered")
}
