package lsp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// ---------------------------------------------------------------------------
// ForFile sync.Cond tests
// ---------------------------------------------------------------------------

// newStubClient returns a zero-value *Client for use as a test stand-in.
// It does not start any transport — white-box access to Manager fields means
// we never need a real connection for these state-machine tests.
func newStubClient(_ *testing.T) *Client {
	return &Client{}
}

// TestForFile_SignaledBeforeWait verifies the fast path when a predecessor
// goroutine completes *before* ForFile is called: ForFile should return the
// pre-populated client immediately without ever blocking in Wait.
func TestForFile_SignaledBeforeWait(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp")
	stub := newStubClient(t)

	// Goroutine A: simulate a starter that finishes quickly.
	// Set starting, then complete before the test goroutine calls ForFile.
	m.mu.Lock()
	m.starting["go"] = true
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(10 * time.Millisecond)
		m.mu.Lock()
		m.clients["go"] = stub
		delete(m.starting, "go")
		m.cond.Broadcast()
		m.mu.Unlock()
	}()

	// Wait for goroutine A to complete before calling ForFile so the server
	// is already ready — this exercises the "signaled before wait" path.
	<-done

	ctx := context.Background()
	got, lang, err := m.ForFile(ctx, "main.go")
	require.NoError(t, err)
	require.Equal(t, "go", lang)
	require.Equal(t, stub, got)
}

// TestForFile_SignaledDuringWait verifies that a waiter already blocked in
// Wait wakes correctly when the starter broadcasts.
func TestForFile_SignaledDuringWait(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp")
	stub := newStubClient(t)

	// Set starting[go] = true while holding the lock so ForFile will Wait.
	m.mu.Lock()
	m.starting["go"] = true
	m.mu.Unlock()

	result := make(chan error, 1)
	var gotClient *Client
	go func() {
		ctx := context.Background()
		c, _, err := m.ForFile(ctx, "main.go")
		gotClient = c
		result <- err
	}()

	// Give the goroutine time to enter Wait before we broadcast.
	time.Sleep(50 * time.Millisecond)

	m.mu.Lock()
	m.clients["go"] = stub
	delete(m.starting, "go")
	m.cond.Broadcast()
	m.mu.Unlock()

	select {
	case err := <-result:
		require.NoError(t, err)
		require.Equal(t, stub, gotClient)
	case <-time.After(2 * time.Second):
		t.Fatal("ForFile did not return after Broadcast")
	}
}

// TestForFile_PredecessorFailureFallthrough verifies that when a predecessor
// starter finishes without populating clients[lang], the waiter falls through
// to promote itself and attempt its own start. Uses "java" (jdtls) which is
// not present on the test PATH, so startServer returns "found in PATH" error,
// proving the waiter promoted itself rather than returning a nil-client.
func TestForFile_PredecessorFailureFallthrough(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp")

	// Simulate a predecessor that has set starting["java"] = true.
	m.mu.Lock()
	m.starting["java"] = true
	m.mu.Unlock()

	result := make(chan error, 1)
	go func() {
		ctx := context.Background()
		_, _, err := m.ForFile(ctx, "Main.java")
		result <- err
	}()

	// Give the goroutine time to enter Wait.
	time.Sleep(50 * time.Millisecond)

	// Predecessor fails: deletes starting but does NOT set clients["java"].
	m.mu.Lock()
	delete(m.starting, "java")
	m.cond.Broadcast()
	m.mu.Unlock()

	select {
	case err := <-result:
		// Waiter promoted itself and called startServer, which returns
		// "no java language server found in PATH" (jdtls not installed).
		require.Error(t, err)
		require.Contains(t, err.Error(), "found in PATH",
			"waiter should have promoted to starter and hit startServer error")
	case <-time.After(5 * time.Second):
		t.Fatal("ForFile did not return after predecessor failure broadcast")
	}
}

// TestForFile_ContextCanceledDuringWait verifies that cancelling the context
// while ForFile is blocked in Wait causes it to return a context.Canceled error.
func TestForFile_ContextCanceledDuringWait(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp")

	m.mu.Lock()
	m.starting["go"] = true
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	result := make(chan error, 1)
	go func() {
		_, _, err := m.ForFile(ctx, "main.go")
		result <- err
	}()

	// Give the goroutine time to enter Wait.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled),
			"expected context.Canceled, got: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ForFile did not return within 500ms after context cancel")
	}
}

// TestForFile_ContextAlreadyCanceled verifies that ForFile returns immediately
// with context.Canceled when the context is already cancelled at call time.
func TestForFile_ContextAlreadyCanceled(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp")

	m.mu.Lock()
	m.starting["go"] = true
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling ForFile

	start := time.Now()
	_, _, err := m.ForFile(ctx, "main.go")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
	// Should return without blocking — allow generous 200ms for scheduler jitter.
	require.Less(t, elapsed, 200*time.Millisecond,
		"ForFile should return immediately with a pre-cancelled context")
}

// TestForFile_FastPathUnchanged verifies that a server already present in
// clients[lang] is returned immediately without any locking or waiting.
func TestForFile_FastPathUnchanged(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp")
	stub := newStubClient(t)

	m.mu.Lock()
	m.clients["go"] = stub
	m.mu.Unlock()

	ctx := context.Background()
	got, lang, err := m.ForFile(ctx, "main.go")
	require.NoError(t, err)
	require.Equal(t, "go", lang)
	require.Equal(t, stub, got)
}

// TestForFile_NoDeadlockConcurrentLanguages verifies that a waiter for one
// language (go) does not block a caller for a different language (python)
// whose server is already ready.
func TestForFile_NoDeadlockConcurrentLanguages(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp")
	stubPy := newStubClient(t)

	// go is starting (will never complete in this test).
	m.mu.Lock()
	m.starting["go"] = true
	// python is already ready.
	m.clients["python"] = stubPy
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Goroutine A: waits forever on go (we never broadcast for it).
	goResult := make(chan error, 1)
	go func() {
		_, _, err := m.ForFile(ctx, "main.go")
		goResult <- err
	}()

	// Goroutine B: asks for python, should return immediately.
	got, lang, err := m.ForFile(context.Background(), "script.py")
	require.NoError(t, err)
	require.Equal(t, "python", lang)
	require.Equal(t, stubPy, got)

	// Cancel goroutine A's context so it exits cleanly.
	cancel()

	select {
	case err := <-goResult:
		require.True(t, errors.Is(err, context.Canceled),
			"goroutine A should exit with context.Canceled, got: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("goroutine A (go waiter) did not exit after context cancel")
	}
}
