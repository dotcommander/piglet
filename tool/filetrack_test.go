package tool

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileTracker_UnreadFileNotStale(t *testing.T) {
	t.Parallel()
	ft := &fileTracker{reads: make(map[string]time.Time)}
	assert.Empty(t, ft.CheckStale("/nonexistent/file.txt"))
}

func TestFileTracker_UnchangedFileNotStale(t *testing.T) {
	t.Parallel()
	ft := &fileTracker{reads: make(map[string]time.Time)}

	tmp := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(tmp, []byte("hello"), 0644))

	info, err := os.Stat(tmp)
	require.NoError(t, err)
	ft.RecordRead(tmp, info.ModTime())

	assert.Empty(t, ft.CheckStale(tmp))
}

func TestFileTracker_ModifiedFileIsStale(t *testing.T) {
	t.Parallel()
	ft := &fileTracker{reads: make(map[string]time.Time)}

	tmp := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(tmp, []byte("hello"), 0644))

	// Record a read time in the past.
	ft.RecordRead(tmp, time.Now().Add(-2*time.Second))

	// Rewrite so mtime advances.
	require.NoError(t, os.WriteFile(tmp, []byte("changed"), 0644))

	msg := ft.CheckStale(tmp)
	assert.Contains(t, msg, "modified since last read")
}

func TestFileTracker_ClearRemovesTracking(t *testing.T) {
	t.Parallel()
	ft := &fileTracker{reads: make(map[string]time.Time)}

	ft.RecordRead("/some/file.txt", time.Now())
	ft.Clear("/some/file.txt")
	assert.Empty(t, ft.CheckStale("/some/file.txt"))
}

func TestFileTracker_DeletedFileNotStale(t *testing.T) {
	t.Parallel()
	ft := &fileTracker{reads: make(map[string]time.Time)}

	tmp := filepath.Join(t.TempDir(), "gone.txt")
	require.NoError(t, os.WriteFile(tmp, []byte("hello"), 0644))

	info, err := os.Stat(tmp)
	require.NoError(t, err)
	ft.RecordRead(tmp, info.ModTime())

	require.NoError(t, os.Remove(tmp))
	assert.Empty(t, ft.CheckStale(tmp)) // gone — let caller handle
}
