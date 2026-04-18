package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// LanguageForFile
// ---------------------------------------------------------------------------

// TestLanguageForFile verifies every extension in the lookup table and
// confirms unknown extensions return "".
func TestLanguageForFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file string
		want string
	}{
		// Go
		{"main.go", "go"},
		{"util_test.go", "go"},
		// TypeScript
		{"app.ts", "typescript"},
		{"component.tsx", "typescript"},
		// JavaScript
		{"index.js", "javascript"},
		{"app.jsx", "javascript"},
		// Python
		{"script.py", "python"},
		// Rust
		{"lib.rs", "rust"},
		// C / C++
		{"main.c", "c"},
		{"header.h", "c"},
		{"module.cpp", "cpp"},
		{"module.cc", "cpp"},
		{"module.cxx", "cpp"},
		{"header.hpp", "cpp"},
		// Java
		{"Main.java", "java"},
		// Lua / Zig
		{"init.lua", "lua"},
		{"main.zig", "zig"},
		// Case-insensitive extension
		{"MAIN.GO", "go"},
		{"App.TS", "typescript"},
		// Unknown
		{"README.md", ""},
		{"Makefile", ""},
		{"data.json", ""},
		{"", ""},
		// Path with directories — only extension matters
		{"/some/deep/path/main.go", "go"},
		{"../relative/src.rs", "rust"},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, LanguageForFile(tc.file))
		})
	}
}

// ---------------------------------------------------------------------------
// NewManager / Manager.CWD
// ---------------------------------------------------------------------------

// TestNewManagerCWD verifies that NewManager stores cwd and CWD() returns it.
func TestNewManagerCWD(t *testing.T) {
	t.Parallel()

	cases := []string{
		"/tmp/project",
		"/home/user/code",
		"",
	}
	for _, cwd := range cases {
		t.Run(cwd, func(t *testing.T) {
			t.Parallel()
			m := NewManager(cwd)
			require.Equal(t, cwd, m.CWD())
		})
	}
}

// TestNewManagerInternalMaps verifies that the manager is constructed with
// non-nil internal maps (prevents nil-map panics on first use).
func TestNewManagerInternalMaps(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp")
	require.NotNil(t, m.clients)
	require.NotNil(t, m.starting)
	require.NotNil(t, m.openFiles)
}

// ---------------------------------------------------------------------------
// serverNames (unexported)
// ---------------------------------------------------------------------------

// TestServerNames verifies that serverNames joins command names correctly.
func TestServerNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		configs []ServerConfig
		want    string
	}{
		{
			name:    "single",
			configs: []ServerConfig{{Command: "gopls"}},
			want:    "gopls",
		},
		{
			name: "multiple",
			configs: []ServerConfig{
				{Command: "pylsp"},
				{Command: "pyright-langserver", Args: []string{"--stdio"}},
			},
			want: "pylsp, pyright-langserver",
		},
		{
			name:    "empty",
			configs: []ServerConfig{},
			want:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, serverNames(tc.configs))
		})
	}
}

// ---------------------------------------------------------------------------
// FindSymbolColumn
// ---------------------------------------------------------------------------

// TestFindSymbolColumn verifies column detection with ASCII and multi-byte content.
func TestFindSymbolColumn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fpath := filepath.Join(dir, "sym.go")

	content := strings.Join([]string{
		"package main",            // line 0
		"",                        // line 1
		"func myFunc() {}",        // line 2 — "myFunc" starts at col 5
		"var x = someOtherFunc()", // line 3 — "someOtherFunc" starts at col 8
		"// café unicode",         // line 4 — "café" at col 3 (rune-based)
	}, "\n")
	require.NoError(t, os.WriteFile(fpath, []byte(content), 0o600))

	cases := []struct {
		name    string
		line    int
		symbol  string
		wantCol int
		wantErr bool
	}{
		{"func name", 2, "myFunc", 5, false},
		{"var on line 3", 3, "someOtherFunc", 8, false},
		{"symbol not found", 0, "notHere", 0, true},
		{"line out of range high", 99, "anything", 0, true},
		{"line out of range low", -1, "anything", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			col, err := FindSymbolColumn(fpath, tc.line, tc.symbol)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantCol, col)
		})
	}
}

// TestFindSymbolColumnFileNotFound verifies a clear error when the file is absent.
func TestFindSymbolColumnFileNotFound(t *testing.T) {
	t.Parallel()

	_, err := FindSymbolColumn("/nonexistent/path/file.go", 0, "sym")
	require.Error(t, err)
}

// TestFindSymbolColumnUnicode verifies that column is measured in runes, not bytes.
func TestFindSymbolColumnUnicode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fpath := filepath.Join(dir, "unicode.go")

	// "café target" — "café" is 4 runes but 5 bytes (é = 2 bytes in UTF-8)
	// "target" starts at rune index 5 (after "café ")
	require.NoError(t, os.WriteFile(fpath, []byte("café target\n"), 0o600))

	col, err := FindSymbolColumn(fpath, 0, "target")
	require.NoError(t, err)
	// rune count of "café " is 5
	require.Equal(t, 5, col)
}
