package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
)

// ---------------------------------------------------------------------------
// Tool result helpers
// ---------------------------------------------------------------------------

func textResult(text string) *core.ToolResult {
	return &core.ToolResult{
		Content: []core.ContentBlock{core.TextContent{Text: text}},
	}
}

// ---------------------------------------------------------------------------
// Argument extraction (JSON numbers decode as float64)
// ---------------------------------------------------------------------------

func stringArg(args map[string]any, key, fallback string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func intArg(args map[string]any, key string, fallback int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return fallback
	}
}

func boolArg(args map[string]any, key string, fallback bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return fallback
}

func requirePath(args map[string]any, cwd string) (string, *core.ToolResult) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", textResult("error: path is required")
	}
	return resolvePath(cwd, path), nil
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

func resolvePath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

func atomicWrite(path string, data []byte) error {
	return config.AtomicWrite(path, data, 0644)
}

// ---------------------------------------------------------------------------
// Directory filtering — shared by find (via filteredFS) and grep (via walk)
// ---------------------------------------------------------------------------

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".next", "__pycache__", ".cache", "dist", "build":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

// filteredFS wraps an fs.FS and hides directories matching the skip predicate.
// Used by find's doublestar.GlobWalk to prevent recursion into junk directories.
type filteredFS struct {
	base fs.FS
	skip func(string) bool
}

func (f filteredFS) Open(name string) (fs.File, error) { return f.base.Open(name) }

func (f filteredFS) ReadDir(name string) ([]fs.DirEntry, error) {
	rdfs, ok := f.base.(fs.ReadDirFS)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	entries, err := rdfs.ReadDir(name)
	if err != nil {
		return nil, err
	}
	filtered := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() && f.skip(e.Name()) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered, nil
}

// ---------------------------------------------------------------------------
// Bounded I/O — caps memory during exec.Cmd output capture
// ---------------------------------------------------------------------------

// boundedWriter caps memory usage by discarding writes beyond the limit.
// Always returns len(p), nil so exec.Cmd pipes don't break.
type boundedWriter struct {
	buf   strings.Builder
	limit int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if w.buf.Len() >= w.limit {
		return len(p), nil
	}
	rem := w.limit - w.buf.Len()
	if len(p) > rem {
		w.buf.Write(p[:rem])
	} else {
		w.buf.Write(p)
	}
	return len(p), nil
}

func (w *boundedWriter) String() string  { return w.buf.String() }
func (w *boundedWriter) Len() int        { return w.buf.Len() }
func (w *boundedWriter) Truncated() bool { return w.buf.Len() >= w.limit }

// ---------------------------------------------------------------------------
// Undo snapshots
// ---------------------------------------------------------------------------

// snapshotFile saves a copy of the file at path to the undo directory.
// Uses ~/.config/piglet/undo/ with a hash of the file path as the filename.
// Only the most recent snapshot per file is kept (overwritten on each write).
// Errors are silently ignored — undo is best-effort.
func snapshotFile(path string) {
	info, err := os.Stat(path)
	const maxSnapshotSize = 10 << 20 // 10 MB
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSnapshotSize {
		return // new file, non-regular, or too large to snapshot
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	dir, err := undoDir()
	if err != nil {
		return
	}

	// Use sha256 of absolute path as filename to avoid path separator issues
	h := sha256.Sum256([]byte(path))
	name := hex.EncodeToString(h[:16]) // 32-char hex
	snapPath := filepath.Join(dir, name+".snap")

	// Skip if snapshot already exists with same content
	existing, err := os.ReadFile(snapPath)
	if err == nil && bytes.Equal(existing, data) {
		return
	}

	// Write snapshot: <hash>.snap (content) + <hash>.path (original path)
	_ = config.AtomicWrite(snapPath, data, 0600)
	_ = config.AtomicWrite(filepath.Join(dir, name+".path"), []byte(path), 0600)
}

// undoDir returns ~/.config/piglet/undo/, creating it if needed.
func undoDir() (string, error) {
	cfgDir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfgDir, "undo")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// UndoSnapshots returns all undo snapshots as path→content pairs.
func UndoSnapshots() (map[string][]byte, error) {
	dir, err := undoDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]byte)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".path") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".path")
		pathData, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		snapData, err := os.ReadFile(filepath.Join(dir, base+".snap"))
		if err != nil {
			continue
		}
		result[string(pathData)] = snapData
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Display formatting
// ---------------------------------------------------------------------------

func FormatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
