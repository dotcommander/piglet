package tool

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/errfmt"
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

func requirePath(args map[string]any, cwd string) (string, *core.ToolResult) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", errfmt.ToolErr(errfmt.ToolErrInvalidArgs, "path is required", "provide an absolute file path")
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

// lineTailWriter wraps an io.Writer (the bounded capture sink) and invokes
// onLine with each completed newline-terminated line as it streams through.
// It is the streaming tap for the bash tool's live tail line — the wrapped
// writer still receives every byte, so capture semantics are unchanged.
//
// Not safe for concurrent Write calls; exec.Cmd serializes writes per stream.
type lineTailWriter struct {
	dst     interface{ Write([]byte) (int, error) }
	onLine  func(string)
	pending strings.Builder
}

func (w *lineTailWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	// Tap regardless of how many bytes dst kept — the live tail must see
	// output even after the bounded sink is full.
	for _, b := range p {
		if b == '\n' {
			w.flushLine()
			continue
		}
		w.pending.WriteByte(b)
	}
	return n, err
}

// flushLine emits the buffered partial line if non-empty, then resets it.
func (w *lineTailWriter) flushLine() {
	line := strings.TrimRight(w.pending.String(), "\r")
	w.pending.Reset()
	if line != "" && w.onLine != nil {
		w.onLine(line)
	}
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
