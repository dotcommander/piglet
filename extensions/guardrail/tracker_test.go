package guardrail

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdd_Accumulates(t *testing.T) {
	t.Parallel()
	tr := NewTracker(filepath.Join(t.TempDir(), "usage.json"))
	got := tr.Add(100, 200)
	assert.Equal(t, int64(300), got)
	got = tr.Add(50, 0)
	assert.Equal(t, int64(350), got)
	assert.Equal(t, int64(350), tr.Used())
}

func TestAdd_DateRollover(t *testing.T) {
	t.Parallel()
	yesterday := time.Now().AddDate(0, 0, -1)
	today := time.Now()

	tr := newTrackerWithClock(filepath.Join(t.TempDir(), "usage.json"), func() time.Time { return yesterday })
	// Seed with yesterday's data.
	tr.Add(9999, 0)
	assert.Equal(t, int64(9999), tr.Used())

	// Advance clock to today — next Add should reset.
	tr.nowFn = func() time.Time { return today }
	got := tr.Add(10, 20)
	assert.Equal(t, int64(30), got, "should reset to 30, not accumulate on top of yesterday")
	assert.Equal(t, today.Format("2006-01-02"), tr.Date())
}

func TestUsed_ReflectsRolloverWithoutAdd(t *testing.T) {
	t.Parallel()
	yesterday := time.Now().AddDate(0, 0, -1)
	today := time.Now()

	tr := newTrackerWithClock(filepath.Join(t.TempDir(), "usage.json"), func() time.Time { return yesterday })
	tr.Add(9999, 0)

	tr.nowFn = func() time.Time { return today }
	// No Add called — Used() should still reflect rollover.
	assert.Equal(t, int64(0), tr.Used())
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.json")

	tr1 := NewTracker(path)
	tr1.Add(1, 2)
	require.NoError(t, tr1.SaveTo(path))

	tr2 := NewTracker(path)
	require.NoError(t, tr2.LoadFrom(path))
	assert.Equal(t, int64(3), tr2.Used())
	assert.Equal(t, time.Now().Format("2006-01-02"), tr2.Date())
}

func TestLoad_OldDateDiscarded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.json")

	// Write a file with yesterday's date.
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	data := []byte(`{"date":"` + yesterday + `","used":999}`)
	require.NoError(t, os.WriteFile(path, data, 0600))

	tr := NewTracker(path)
	require.NoError(t, tr.LoadFrom(path))
	assert.Equal(t, int64(0), tr.Used(), "stale usage should be zeroed")
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()
	tr := NewTracker(filepath.Join(t.TempDir(), "nonexistent.json"))
	err := tr.LoadFrom(tr.Path())
	require.NoError(t, err)
	assert.Equal(t, int64(0), tr.Used())
}

func TestLoad_CorruptJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0600))

	tr := NewTracker(path)
	err := tr.LoadFrom(path)
	require.Error(t, err, "corrupt JSON should return an error")
	assert.Equal(t, int64(0), tr.Used(), "tracker should remain zeroed after corrupt load")
}

func TestAdd_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	tr := NewTracker(filepath.Join(t.TempDir(), "usage.json"))

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			tr.Add(1, 1)
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(200), tr.Used())
}

func TestSaveTo_AtomicWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.json")

	tr := NewTracker(path)
	tr.Add(42, 0)
	require.NoError(t, tr.SaveTo(path))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, filepath.Ext(e.Name()) == ".tmp", "no leftover .tmp files: found %s", e.Name())
	}
	assert.FileExists(t, path)
}
