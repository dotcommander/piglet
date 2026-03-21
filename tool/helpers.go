package tool

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
	tmp := path + ".piglet-tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
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
	filtered := entries[:0]
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

func (w *boundedWriter) String() string { return w.buf.String() }
func (w *boundedWriter) Len() int       { return w.buf.Len() }
func (w *boundedWriter) Truncated() bool { return w.buf.Len() >= w.limit }

// ---------------------------------------------------------------------------
// Display formatting
// ---------------------------------------------------------------------------

func formatSize(bytes int64) string {
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
