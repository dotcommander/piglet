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

// TestFileTracker_SymlinkBypass verifies that a read recorded via a symlink
// is detected as stale when CheckStale is called with the canonical path,
// and vice versa — closing the symlink canonicalization bypass.
func TestFileTracker_SymlinkBypass(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	canonical := filepath.Join(dir, "real.txt")
	symlink := filepath.Join(dir, "link.txt")

	require.NoError(t, os.WriteFile(canonical, []byte("original"), 0644))
	require.NoError(t, os.Symlink(canonical, symlink))

	ft := &fileTracker{reads: make(map[string]time.Time)}

	// Read is recorded via symlink path.
	info, err := os.Stat(symlink)
	require.NoError(t, err)
	ft.RecordRead(symlink, info.ModTime())

	// Advance mtime by rewriting via canonical path.
	require.NoError(t, os.WriteFile(canonical, []byte("modified"), 0644))

	// CheckStale via canonical path must detect the stale read.
	msg := ft.CheckStale(canonical)
	assert.Contains(t, msg, "modified since last read",
		"read via symlink should be detected as stale when checked via canonical path")

	// Inverse: read recorded via canonical, checked via symlink — also stale.
	ft2 := &fileTracker{reads: make(map[string]time.Time)}
	info2, err := os.Stat(canonical)
	require.NoError(t, err)
	// Record with the pre-modify mtime to simulate a prior read.
	ft2.RecordRead(canonical, info2.ModTime().Add(-time.Second))

	msg2 := ft2.CheckStale(symlink)
	assert.Contains(t, msg2, "modified since last read",
		"read via canonical should be detected as stale when checked via symlink")
}
