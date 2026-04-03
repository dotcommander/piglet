package confirm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// capOutput
// ---------------------------------------------------------------------------

func TestCapOutput_Short(t *testing.T) {
	input := []byte("hello")
	got := capOutput(input, 100)
	assert.Equal(t, "hello", got)
}

func TestCapOutput_Exact(t *testing.T) {
	input := []byte("hello")
	got := capOutput(input, 5)
	assert.Equal(t, "hello", got)
}

func TestCapOutput_Long(t *testing.T) {
	input := []byte("hello world")
	got := capOutput(input, 5)
	assert.Equal(t, "hello", got)
}

func TestCapOutput_Whitespace(t *testing.T) {
	input := []byte("  \n  trimmed  \n  ")
	got := capOutput(input, 1024)
	assert.Equal(t, "trimmed", got)
}

// ---------------------------------------------------------------------------
// FormatVerdict
// ---------------------------------------------------------------------------

func TestFormatVerdict_NoFiles(t *testing.T) {
	r := &Result{Pass: true, Files: nil}
	got := FormatVerdict(r)
	assert.Contains(t, got, "nothing to verify (no changes detected)")
}

func TestFormatVerdict_NoChecks(t *testing.T) {
	r := &Result{Pass: true, Files: []string{"a.go"}, Checks: nil}
	got := FormatVerdict(r)
	assert.Contains(t, got, "nothing to verify (no Go packages affected)")
}

func TestFormatVerdict_AllPass(t *testing.T) {
	r := &Result{
		Pass:     true,
		Files:    []string{"a.go"},
		Packages: []string{"pkg"},
		Checks: []CheckResult{
			{Name: "typecheck", Pass: true, Elapsed: 0.5},
		},
		Elapsed: 0.5,
	}
	got := FormatVerdict(r)
	assert.True(t, strings.HasPrefix(got, "PASS"), "expected PASS prefix, got: %q", got)
	assert.Contains(t, got, "✓")
	assert.NotContains(t, got, "✗")
}

func TestFormatVerdict_OneFails(t *testing.T) {
	r := &Result{
		Pass:     false,
		Files:    []string{"a.go"},
		Packages: []string{"pkg"},
		Checks: []CheckResult{
			{Name: "typecheck", Pass: true, Elapsed: 0.1},
			{Name: "test", Pass: false, Output: "FAIL\t./...", Elapsed: 1.2},
		},
		Elapsed: 1.3,
	}
	got := FormatVerdict(r)
	assert.True(t, strings.HasPrefix(got, "FAIL"), "expected FAIL prefix, got: %q", got)
	assert.Contains(t, got, "✗")
	assert.Contains(t, got, "--- output ---")
}

// ---------------------------------------------------------------------------
// allSourceFiles / allImports (affected.go)
// ---------------------------------------------------------------------------

func TestAllSourceFiles(t *testing.T) {
	e := goListEntry{
		GoFiles:      []string{"a.go", "b.go"},
		TestGoFiles:  []string{"a_test.go"},
		XTestGoFiles: []string{"b_test.go"},
	}
	got := allSourceFiles(e)
	require.Len(t, got, 4)
	assert.Equal(t, []string{"a.go", "b.go", "a_test.go", "b_test.go"}, got)
}

func TestAllImports(t *testing.T) {
	e := goListEntry{
		Imports:      []string{"fmt", "os"},
		TestImports:  []string{"testing"},
		XTestImports: []string{"github.com/stretchr/testify/assert"},
	}
	got := allImports(e)
	require.Len(t, got, 4)
	assert.Equal(t, []string{"fmt", "os", "testing", "github.com/stretchr/testify/assert"}, got)
}

// empty entries return empty slices (not nil, but len 0 is fine either way)
func TestAllSourceFiles_Empty(t *testing.T) {
	got := allSourceFiles(goListEntry{})
	assert.Empty(t, got)
}

func TestAllImports_Empty(t *testing.T) {
	got := allImports(goListEntry{})
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// Run — no changes (explicit empty file list)
// ---------------------------------------------------------------------------

func TestRun_NoChanges(t *testing.T) {
	r, err := Run(Options{Files: []string{}})
	require.NoError(t, err)
	assert.True(t, r.Pass)
	assert.Empty(t, r.Checks)
}

// ---------------------------------------------------------------------------
// TypeCheck / RunTests / Lint — empty input fast-paths
// ---------------------------------------------------------------------------

func TestTypeCheck_EmptyPackages(t *testing.T) {
	got := TypeCheck(nil, "")
	assert.True(t, got.Pass)
	assert.Equal(t, "typecheck", got.Name)
}

func TestRunTests_EmptyPackages(t *testing.T) {
	got := RunTests(nil, "")
	assert.True(t, got.Pass)
	assert.Equal(t, "test", got.Name)
}

func TestLint_EmptyFiles(t *testing.T) {
	got := Lint(nil, "")
	assert.True(t, got.Pass)
	assert.Equal(t, "lint", got.Name)
}
