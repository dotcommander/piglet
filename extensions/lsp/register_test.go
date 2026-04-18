package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// resolveFile
// ---------------------------------------------------------------------------

// TestResolveFileAbsolute verifies that an absolute path is returned unchanged.
func TestResolveFileAbsolute(t *testing.T) {
	t.Parallel()

	m := NewManager("/project")
	args := map[string]any{"file": "/absolute/path/main.go"}
	require.Equal(t, "/absolute/path/main.go", resolveFile(m, args))
}

// TestResolveFileRelative verifies that a relative path is joined with CWD.
func TestResolveFileRelative(t *testing.T) {
	t.Parallel()

	m := NewManager("/project")
	args := map[string]any{"file": "internal/foo.go"}
	got := resolveFile(m, args)
	require.Equal(t, filepath.Join("/project", "internal/foo.go"), got)
}

// TestResolveFileMissing verifies that a missing "file" key returns "".
func TestResolveFileMissing(t *testing.T) {
	t.Parallel()

	m := NewManager("/project")
	require.Equal(t, "", resolveFile(m, map[string]any{}))
	require.Equal(t, "", resolveFile(m, map[string]any{"file": ""}))
}

// TestResolveFileNilManager verifies that a nil manager is safe and absolute
// paths still pass through.
func TestResolveFileNilManager(t *testing.T) {
	t.Parallel()

	args := map[string]any{"file": "/abs/path.go"}
	require.Equal(t, "/abs/path.go", resolveFile(nil, args))
}

// TestResolveFileRelativeNilManager verifies that a relative path with a nil
// manager returns the path unmodified (no join can be performed).
func TestResolveFileRelativeNilManager(t *testing.T) {
	t.Parallel()

	args := map[string]any{"file": "relative/path.go"}
	got := resolveFile(nil, args)
	// With nil manager filepath.IsAbs is false and the if-branch is skipped,
	// so the raw relative path is returned.
	require.Equal(t, "relative/path.go", got)
}

// ---------------------------------------------------------------------------
// resolvePosition
// ---------------------------------------------------------------------------

// TestResolvePositionWithColumn verifies 1-based → 0-based conversion when
// an explicit column is provided.
func TestResolvePositionWithColumn(t *testing.T) {
	t.Parallel()

	m := NewManager("/project")
	args := map[string]any{
		"file":   "/project/main.go",
		"line":   float64(5),
		"column": float64(3),
	}

	file, line, col, err := resolvePosition(m, args)
	require.NoError(t, err)
	require.Equal(t, "/project/main.go", file)
	require.Equal(t, 4, line) // 5-1
	require.Equal(t, 2, col)  // 3-1
}

// TestResolvePositionDefaultsColumnZero verifies that when no column or symbol
// is provided, col defaults to 0.
func TestResolvePositionDefaultsColumnZero(t *testing.T) {
	t.Parallel()

	m := NewManager("/project")
	args := map[string]any{
		"file": "/project/main.go",
		"line": float64(1),
	}

	_, _, col, err := resolvePosition(m, args)
	require.NoError(t, err)
	require.Equal(t, 0, col)
}

// TestResolvePositionMissingFile verifies that missing file returns an error.
func TestResolvePositionMissingFile(t *testing.T) {
	t.Parallel()

	m := NewManager("/project")
	args := map[string]any{"line": float64(1)}

	_, _, _, err := resolvePosition(m, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "file is required")
}

// TestResolvePositionLineZero verifies that line=0 (would give 0-1=-1) is
// rejected with a clear error.
func TestResolvePositionLineZero(t *testing.T) {
	t.Parallel()

	m := NewManager("/project")
	args := map[string]any{
		"file": "/project/main.go",
		"line": float64(0),
	}

	_, _, _, err := resolvePosition(m, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "line must be >= 1")
}

// TestResolvePositionWithSymbol verifies that the symbol lookup path is taken
// when no explicit column is provided but a symbol is given — uses a real
// temp file so FindSymbolColumn can do its work.
func TestResolvePositionWithSymbol(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fpath := filepath.Join(dir, "pos.go")
	content := strings.Join([]string{
		"package main",       // line 1
		"func targetFn() {}", // line 2 — "targetFn" at col 5
		"}",
	}, "\n")
	require.NoError(t, os.WriteFile(fpath, []byte(content), 0o600))

	m := NewManager(dir)
	args := map[string]any{
		"file":   fpath,
		"line":   float64(2),
		"symbol": "targetFn",
	}

	file, line, col, err := resolvePosition(m, args)
	require.NoError(t, err)
	require.Equal(t, fpath, file)
	require.Equal(t, 1, line) // 2-1
	require.Equal(t, 5, col)  // "func " is 5 bytes/runes
}

// TestResolvePositionColumnTakesPrecedenceOverSymbol verifies that an explicit
// column wins over a symbol argument (early return in the function).
func TestResolvePositionColumnTakesPrecedenceOverSymbol(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fpath := filepath.Join(dir, "prio.go")
	require.NoError(t, os.WriteFile(fpath, []byte("func foo() {}\n"), 0o600))

	m := NewManager(dir)
	args := map[string]any{
		"file":   fpath,
		"line":   float64(1),
		"column": float64(7), // explicit col takes priority
		"symbol": "foo",      // would give col=5 if used
	}

	_, _, col, err := resolvePosition(m, args)
	require.NoError(t, err)
	require.Equal(t, 6, col) // 7-1, not 5
}
