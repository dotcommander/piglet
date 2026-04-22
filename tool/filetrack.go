package tool

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// fileTracker records the mtime of files when they are read.
// The edit and write tools check this before writing to detect
// concurrent modifications (TOCTOU staleness).
//
// All keys are canonical paths (EvalSymlinks resolved). This prevents a
// read-via-symlink / write-to-canonical bypass of the staleness guard.
type fileTracker struct {
	mu    sync.RWMutex
	reads map[string]time.Time // canonical path → mtime at last read
}

// canonicalize resolves symlinks in path so map keys are always canonical.
// If the file does not exist yet (new-file write path), it falls back to
// resolving the parent directory and appending the base name.
// On any remaining error it returns the original path unchanged — the
// fallback preserves existing behaviour rather than silently dropping the entry.
func canonicalize(path string) string {
	if c, err := filepath.EvalSymlinks(path); err == nil {
		return c
	}
	// File may not exist yet — try canonicalizing the parent.
	if c, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(c, filepath.Base(path))
	}
	return path
}

// RecordRead saves the mtime of a file after a successful read.
func (t *fileTracker) RecordRead(path string, mtime time.Time) {
	t.mu.Lock()
	t.reads[canonicalize(path)] = mtime
	t.mu.Unlock()
}

// CheckStale returns an error message if the file has been modified since
// the last read. Returns "" if the file was never read (new file) or if
// the content is unchanged (handles cloud sync mtime jitter).
func (t *fileTracker) CheckStale(path string) string {
	key := canonicalize(path)

	t.mu.RLock()
	readTime, tracked := t.reads[key]
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
	delete(t.reads, canonicalize(path))
	t.mu.Unlock()
}
