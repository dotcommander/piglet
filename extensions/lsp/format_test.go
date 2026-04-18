package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// FormatHover
// ---------------------------------------------------------------------------

// TestFormatHoverNilAndEmpty verifies the "no info" sentinel text.
func TestFormatHoverNilAndEmpty(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "No hover information available.", FormatHover(nil))
	})

	t.Run("empty value", func(t *testing.T) {
		t.Parallel()
		h := &HoverResult{Contents: MarkupContent{Kind: "plaintext", Value: ""}}
		require.Equal(t, "No hover information available.", FormatHover(h))
	})
}

// TestFormatHoverContent verifies that non-empty content is returned verbatim.
func TestFormatHoverContent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		kind  string
		value string
	}{
		{"plaintext", "plaintext", "func Foo(x int) error"},
		{"markdown", "markdown", "**func** `Bar()` — does something"},
		{"multiline", "markdown", "line1\nline2\nline3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &HoverResult{Contents: MarkupContent{Kind: tc.kind, Value: tc.value}}
			require.Equal(t, tc.value, FormatHover(h))
		})
	}
}

// ---------------------------------------------------------------------------
// FormatWorkspaceEdit
// ---------------------------------------------------------------------------

// TestFormatWorkspaceEditNilAndEmpty verifies the "no changes" sentinel.
func TestFormatWorkspaceEditNilAndEmpty(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "No changes.", FormatWorkspaceEdit(nil, "/cwd"))
	})

	t.Run("empty changes", func(t *testing.T) {
		t.Parallel()
		e := &WorkspaceEdit{Changes: map[string][]TextEdit{}}
		require.Equal(t, "No changes.", FormatWorkspaceEdit(e, "/cwd"))
	})
}

// TestFormatWorkspaceEditSingleFile verifies output for a single-file edit.
func TestFormatWorkspaceEditSingleFile(t *testing.T) {
	t.Parallel()

	cwd := "/project"
	edit := &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			"file:///project/main.go": {
				{
					Range:   Range{Start: Position{Line: 4, Character: 0}, End: Position{Line: 4, Character: 10}},
					NewText: "newContent",
				},
			},
		},
	}

	result := FormatWorkspaceEdit(edit, cwd)

	// Relative path should appear
	require.Contains(t, result, "main.go")
	// Line number (4+1=5)
	require.Contains(t, result, "line 5:")
	// New text quoted
	require.Contains(t, result, `"newContent"`)
	// Summary totals
	require.Contains(t, result, "1 file(s), 1 edit(s) total")
}

// TestFormatWorkspaceEditMultipleFiles verifies total-count accuracy.
func TestFormatWorkspaceEditMultipleFiles(t *testing.T) {
	t.Parallel()

	edit := &WorkspaceEdit{
		Changes: map[string][]TextEdit{
			"file:///a.go": {
				{Range: Range{}, NewText: "x"},
				{Range: Range{}, NewText: "y"},
			},
			"file:///b.go": {
				{Range: Range{}, NewText: "z"},
			},
		},
	}

	result := FormatWorkspaceEdit(edit, "")
	require.Contains(t, result, "2 file(s), 3 edit(s) total")
}

// ---------------------------------------------------------------------------
// FormatSymbols
// ---------------------------------------------------------------------------

// TestFormatSymbolsEmpty verifies the "no symbols" sentinel.
func TestFormatSymbolsEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "No symbols found.", FormatSymbols(nil, "/cwd"))
	require.Equal(t, "No symbols found.", FormatSymbols([]DocumentSymbol{}, "/cwd"))
}

// TestFormatSymbolsFlat verifies that flat symbols render with kind, name, and
// 1-based line number.
func TestFormatSymbolsFlat(t *testing.T) {
	t.Parallel()

	symbols := []DocumentSymbol{
		{
			Name:  "Handler",
			Kind:  SymbolKindFunction,
			Range: Range{Start: Position{Line: 9, Character: 0}, End: Position{Line: 20, Character: 1}},
		},
		{
			Name:  "Config",
			Kind:  SymbolKindStruct,
			Range: Range{Start: Position{Line: 2, Character: 0}, End: Position{Line: 7, Character: 1}},
		},
	}

	result := FormatSymbols(symbols, "/cwd")
	require.Contains(t, result, "Handler")
	require.Contains(t, result, "function")
	require.Contains(t, result, "line 10") // 9+1
	require.Contains(t, result, "Config")
	require.Contains(t, result, "struct")
	require.Contains(t, result, "line 3") // 2+1
}

// TestFormatSymbolsNested verifies that children are indented relative to
// their parent symbol.
func TestFormatSymbolsNested(t *testing.T) {
	t.Parallel()

	child := DocumentSymbol{
		Name:  "childMethod",
		Kind:  SymbolKindMethod,
		Range: Range{Start: Position{Line: 5, Character: 0}},
	}
	parent := DocumentSymbol{
		Name:     "ParentClass",
		Kind:     SymbolKindClass,
		Range:    Range{Start: Position{Line: 1, Character: 0}},
		Children: []DocumentSymbol{child},
	}

	result := FormatSymbols([]DocumentSymbol{parent}, "/cwd")
	lines := strings.Split(result, "\n")

	// Parent must appear before child
	parentIdx := -1
	childIdx := -1
	for i, l := range lines {
		if strings.Contains(l, "ParentClass") {
			parentIdx = i
		}
		if strings.Contains(l, "childMethod") {
			childIdx = i
		}
	}
	require.NotEqual(t, -1, parentIdx, "parent line not found")
	require.NotEqual(t, -1, childIdx, "child line not found")
	require.Less(t, parentIdx, childIdx, "parent should appear before child")

	// Child line must have more leading whitespace (indent) than parent
	parentIndent := len(lines[parentIdx]) - len(strings.TrimLeft(lines[parentIdx], " "))
	childIndent := len(lines[childIdx]) - len(strings.TrimLeft(lines[childIdx], " "))
	require.Greater(t, childIndent, parentIndent, "child should be indented more than parent")
}

// ---------------------------------------------------------------------------
// FormatLocations
// ---------------------------------------------------------------------------

// TestFormatLocationsEmpty verifies the "no results" sentinel.
func TestFormatLocationsEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "No results found.", FormatLocations(nil, "/cwd", 2))
	require.Equal(t, "No results found.", FormatLocations([]Location{}, "/cwd", 2))
}

// TestFormatLocationsHeader verifies the count header is present.
func TestFormatLocationsHeader(t *testing.T) {
	t.Parallel()

	// Location with a URI that won't resolve to a real file —
	// context lines will be absent but the path+count header must appear.
	locs := []Location{
		{
			URI:   "file:///nonexistent/path/that/will/not/be/read.go",
			Range: Range{Start: Position{Line: 0, Character: 0}},
		},
		{
			URI:   "file:///another/nonexistent.go",
			Range: Range{Start: Position{Line: 4, Character: 2}},
		},
	}

	result := FormatLocations(locs, "", 0)
	require.Contains(t, result, "2 location(s) found:")
}

// TestFormatLocationsWithContext verifies that context lines are extracted
// from a real temp file and the target line is marked with ">".
func TestFormatLocationsWithContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fpath := filepath.Join(dir, "sample.go")

	lines := []string{
		"package main",      // line 0 → displayed as 1
		"",                  // line 1
		"func main() {",     // line 2 → displayed as 3
		`	fmt.Println("x")`, // line 3 → displayed as 4
		"}",                 // line 4 → displayed as 5
	}
	require.NoError(t, os.WriteFile(fpath, []byte(strings.Join(lines, "\n")), 0o600))

	locs := []Location{
		{
			URI:   pathToURI(fpath),
			Range: Range{Start: Position{Line: 2, Character: 0}}, // "func main() {"
		},
	}

	result := FormatLocations(locs, dir, 1)

	// Header
	require.Contains(t, result, "1 location(s) found:")
	// Relative path should be just the filename (cwd is dir)
	require.Contains(t, result, "sample.go:3")
	// Target line marker
	require.Contains(t, result, "> ")
	// Target line content
	require.Contains(t, result, "func main()")
	// Context line (line before: blank or line after: fmt.Println)
	require.True(t,
		strings.Contains(result, "fmt.Println") || strings.Contains(result, "package main"),
		"expected at least one context line, got:\n%s", result)
}

// ---------------------------------------------------------------------------
// formatContext (unexported) — pure slice-window logic
// ---------------------------------------------------------------------------

// TestFormatContext verifies window clamping and the ">" marker placement.
func TestFormatContext(t *testing.T) {
	t.Parallel()

	src := []string{"aaa", "bbb", "ccc", "ddd", "eee"}

	cases := []struct {
		name        string
		targetLine  int
		contextSize int
		wantMarked  string // substring that must appear with ">" prefix
		wantLines   int    // approximate minimum lines in output
	}{
		{"middle with context", 2, 1, "ccc", 3},
		{"first line clamped", 0, 2, "aaa", 1},
		{"last line clamped", 4, 2, "eee", 3},
		{"zero context", 2, 0, "ccc", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := formatContext(src, tc.targetLine, tc.contextSize)

			// The target line must be marked
			var markerLine string
			for _, l := range strings.Split(result, "\n") {
				if strings.HasPrefix(l, "> ") && strings.Contains(l, tc.wantMarked) {
					markerLine = l
					break
				}
			}
			require.NotEmpty(t, markerLine,
				"expected '> ' marker on line containing %q in:\n%s", tc.wantMarked, result)

			// At least tc.wantLines non-empty lines
			var nonEmpty int
			for _, l := range strings.Split(result, "\n") {
				if strings.TrimSpace(l) != "" {
					nonEmpty++
				}
			}
			require.GreaterOrEqual(t, nonEmpty, tc.wantLines)
		})
	}
}

// TestFormatContextEmpty verifies that an empty slice returns empty string.
func TestFormatContextEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", formatContext(nil, 0, 2))
	require.Equal(t, "", formatContext([]string{}, 0, 2))
}

// ---------------------------------------------------------------------------
// relativePath (unexported)
// ---------------------------------------------------------------------------

// TestRelativePath verifies CWD-relative shortening and fallback behavior.
func TestRelativePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		cwd  string
		want string
	}{
		{
			name: "child of cwd",
			path: "/project/internal/foo.go",
			cwd:  "/project",
			want: "internal/foo.go",
		},
		{
			name: "same dir as cwd",
			path: "/project/main.go",
			cwd:  "/project",
			want: "main.go",
		},
		{
			name: "empty cwd returns path unchanged",
			path: "/absolute/path.go",
			cwd:  "",
			want: "/absolute/path.go",
		},
		{
			name: "sibling directory uses relative dot-dot",
			path: "/a/b/c.go",
			cwd:  "/a/d",
			want: "../b/c.go",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := relativePath(tc.path, tc.cwd)
			// Normalise separators for cross-platform
			require.Equal(t, filepath.FromSlash(tc.want), filepath.FromSlash(got))
		})
	}
}

// TestFormatLocationsLineNumbers verifies that the output line numbers are
// 1-based even when the LSP positions are 0-based.
func TestFormatLocationsLineNumbers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fpath := filepath.Join(dir, "nums.go")
	require.NoError(t, os.WriteFile(fpath, []byte("A\nB\nC\n"), 0o600))

	cases := []struct {
		lspLine  int // 0-based
		wantLine int // 1-based, should appear in output
	}{
		{0, 1},
		{1, 2},
		{2, 3},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("lsp_line_%d", tc.lspLine), func(t *testing.T) {
			t.Parallel()
			locs := []Location{
				{URI: pathToURI(fpath), Range: Range{Start: Position{Line: tc.lspLine}}},
			}
			result := FormatLocations(locs, dir, 0)
			require.Contains(t, result, fmt.Sprintf("nums.go:%d", tc.wantLine))
		})
	}
}
