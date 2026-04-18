package lsp

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPathToURIScheme verifies that pathToURI always produces a "file://" URI.
func TestPathToURIScheme(t *testing.T) {
	t.Parallel()

	cases := []string{
		"/tmp/foo.go",
		"/Users/user/code/main.go",
		"/var/lib/project/x.py",
	}

	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			uri := pathToURI(p)
			require.True(t, strings.HasPrefix(uri, "file://"),
				"expected file:// prefix, got %q", uri)
		})
	}
}

// TestURIRoundTrip verifies uriToPath(pathToURI(path)) == path for absolute
// POSIX paths (only on non-Windows where the test can be deterministic).
func TestURIRoundTrip(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("path round-trip semantics differ on Windows")
	}

	cases := []string{
		"/tmp/foo.go",
		"/var/lib/project/bar.py",
		"/home/user/workspace/main.rs",
	}

	for _, orig := range cases {
		t.Run(orig, func(t *testing.T) {
			t.Parallel()
			uri := pathToURI(orig)
			got := uriToPath(uri)
			require.Equal(t, orig, got)
		})
	}
}

// TestURIToPathMalformed verifies graceful degradation for a non-file URI.
func TestURIToPathMalformed(t *testing.T) {
	t.Parallel()

	// A non-file URI: should not panic; the result may be the raw value minus
	// the "file://" prefix, depending on the fallback branch.
	result := uriToPath("not-a-valid-uri")
	require.NotPanics(t, func() { _ = uriToPath("not-a-valid-uri") })
	_ = result // result is implementation-defined for invalid input
}

// TestPathToURIRelativeInput verifies that relative paths are resolved to
// absolute paths before encoding (so the result is still a valid file URI).
func TestPathToURIRelativeInput(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("absolute path resolution differs on Windows")
	}

	// "." is always resolvable; its absolute form must appear in the URI
	uri := pathToURI(".")
	abs, err := filepath.Abs(".")
	require.NoError(t, err)
	require.Contains(t, uri, abs,
		"relative path should be resolved to absolute in URI")
}
