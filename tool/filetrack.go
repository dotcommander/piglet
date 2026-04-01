package tool

import (
	"os"
	"sync"
	"time"
)

// fileTracker records the mtime of files when they are read.
// The edit and write tools check this before writing to detect
// concurrent modifications (TOCTOU staleness).
type fileTracker struct {
	mu    sync.RWMutex
	reads map[string]time.Time // path → mtime at last read
}

var tracker = &fileTracker{reads: make(map[string]time.Time)}

// RecordRead saves the mtime of a file after a successful read.
func (t *fileTracker) RecordRead(path string, mtime time.Time) {
	t.mu.Lock()
	t.reads[path] = mtime
	t.mu.Unlock()
}

// CheckStale returns an error message if the file has been modified since
// the last read. Returns "" if the file was never read (new file) or if
// the content is unchanged (handles cloud sync mtime jitter).
func (t *fileTracker) CheckStale(path string) string {
	t.mu.RLock()
	readTime, tracked := t.reads[path]
	t.mu.RUnlock()

	if !tracked {
		return "" // never read through our tool — skip check
	}

	info, err := os.Stat(path)
	if err != nil {
		return "" // file gone — let the caller handle it
	}

	if !info.ModTime().After(readTime) {
		return "" // mtime unchanged — safe
	}

	// Mtime changed — could be cloud sync false positive.
	// We don't have the original content cached, so we can't byte-compare.
	// The mtime difference is the best signal we have.
	return "file modified since last read (mtime changed). Re-read the file before editing to confirm the current state."
}

// Clear removes tracking for a path (called after successful write to
// avoid stale-check against our own write).
func (t *fileTracker) Clear(path string) {
	t.mu.Lock()
	delete(t.reads, path)
	t.mu.Unlock()
}
