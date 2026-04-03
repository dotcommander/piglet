package cache_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dotcommander/piglet/extensions/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ns builds a test-unique namespace so parallel tests don't collide.
// The XDG_CONFIG_HOME env is set to t.TempDir() on the parent test, which
// is inherited by non-parallel subtests. For top-level parallel tests we
// use unique namespace prefixes derived from t.Name() and clean up after.
func ns(t *testing.T, suffix string) string {
	t.Helper()
	// Sanitise the test name to a filesystem-safe string.
	name := ""
	for _, r := range t.Name() {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			name += string(r)
		} else {
			name += "_"
		}
	}
	ns := name + "_" + suffix
	t.Cleanup(func() { _ = cache.Purge(ns) })
	return ns
}

func TestGetMiss(t *testing.T) {
	t.Parallel()
	n := ns(t, "miss")

	val, ok := cache.Get(n, "missing-key")
	assert.False(t, ok)
	assert.Equal(t, "", val)
}

func TestSetAndGet(t *testing.T) {
	t.Parallel()
	n := ns(t, "sg")

	require.NoError(t, cache.Set(n, "mykey", "myvalue", time.Hour))

	val, ok := cache.Get(n, "mykey")
	assert.True(t, ok)
	assert.Equal(t, "myvalue", val)
}

func TestExpiry(t *testing.T) {
	t.Parallel()
	n := ns(t, "exp")

	require.NoError(t, cache.Set(n, "expkey", "expvalue", time.Millisecond))
	time.Sleep(5 * time.Millisecond)

	val, ok := cache.Get(n, "expkey")
	assert.False(t, ok)
	assert.Equal(t, "", val)

	// Second get confirms file was lazily deleted.
	val2, ok2 := cache.Get(n, "expkey")
	assert.False(t, ok2)
	assert.Equal(t, "", val2)
}

func TestNoExpiry(t *testing.T) {
	t.Parallel()
	n := ns(t, "noexp")

	require.NoError(t, cache.Set(n, "permkey", "permvalue", 0))

	val, ok := cache.Get(n, "permkey")
	assert.True(t, ok)
	assert.Equal(t, "permvalue", val)
}

func TestDelete(t *testing.T) {
	t.Parallel()
	n := ns(t, "del")

	require.NoError(t, cache.Set(n, "delkey", "delvalue", time.Hour))
	require.NoError(t, cache.Delete(n, "delkey"))

	val, ok := cache.Get(n, "delkey")
	assert.False(t, ok)
	assert.Equal(t, "", val)
}

func TestDeleteMissing(t *testing.T) {
	t.Parallel()
	n := ns(t, "delmiss")

	assert.NoError(t, cache.Delete(n, "no-such-key"))
}

func TestPurge(t *testing.T) {
	t.Parallel()
	// Build the namespace manually so cleanup doesn't fight Purge.
	n := "TestPurge_purge"
	t.Cleanup(func() { _ = cache.Purge(n) })

	keys := []string{"a", "b", "c"}
	for _, k := range keys {
		require.NoError(t, cache.Set(n, k, "v-"+k, time.Hour))
	}
	for _, k := range keys {
		_, ok := cache.Get(n, k)
		require.True(t, ok, "expected key %q before purge", k)
	}

	require.NoError(t, cache.Purge(n))

	for _, k := range keys {
		val, ok := cache.Get(n, k)
		assert.False(t, ok, "key %q should be gone after purge", k)
		assert.Equal(t, "", val)
	}
}

func TestGC(t *testing.T) {
	t.Parallel()
	n := ns(t, "gc")

	// Write 5 entries with slight time gaps so mtime ordering is deterministic.
	keys := []string{"gc1", "gc2", "gc3", "gc4", "gc5"}
	for i, k := range keys {
		require.NoError(t, cache.Set(n, k, fmt.Sprintf("v%d", i), time.Hour))
		time.Sleep(5 * time.Millisecond)
	}

	// GC with maxEntries=3 should remove the 2 oldest (gc1, gc2).
	require.NoError(t, cache.GC(n, 3))

	// Oldest 2 should be gone.
	for _, k := range keys[:2] {
		_, ok := cache.Get(n, k)
		assert.False(t, ok, "key %q should have been GC'd", k)
	}
	// Newest 3 should survive.
	for _, k := range keys[2:] {
		_, ok := cache.Get(n, k)
		assert.True(t, ok, "key %q should have survived GC", k)
	}
}

func TestConcurrentWrites(t *testing.T) {
	t.Parallel()
	n := ns(t, "conc")

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-key-%d", idx)
			errs[idx] = cache.Set(n, key, fmt.Sprintf("value-%d", idx), time.Hour)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d failed", i)
	}

	for i := range goroutines {
		key := fmt.Sprintf("concurrent-key-%d", i)
		val, ok := cache.Get(n, key)
		assert.True(t, ok, "key %q missing after concurrent write", key)
		assert.Equal(t, fmt.Sprintf("value-%d", i), val)
	}
}
