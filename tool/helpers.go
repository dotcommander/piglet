package tool

import (
	"fmt"
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
// Directory filtering — shared by find and grep (via walk)
// ---------------------------------------------------------------------------

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".next", "__pycache__", ".cache", "dist", "build":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
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
