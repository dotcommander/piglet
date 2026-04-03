package fossil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a temp git repo with a known commit history and returns
// the path to the repo directory.
//
// Commit 1: add main.go and util.go
// Commit 2: modify main.go
// Commit 3: modify util.go
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "setup command %v failed: %s", args, out)
	}

	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")

	// Commit 1: initial files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "util.go"),
		[]byte("package main\n\nfunc helper() {}\n"), 0644))
	run("git", "add", "main.go", "util.go")
	run("git", "commit", "-m", "initial commit")

	// Commit 2: modify main.go
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n\n// added line\n"), 0644))
	run("git", "add", "main.go")
	run("git", "commit", "-m", "update main")

	// Commit 3: modify util.go
	require.NoError(t, os.WriteFile(filepath.Join(dir, "util.go"),
		[]byte("package main\n\nfunc helper() {}\n\n// added line\n"), 0644))
	run("git", "add", "util.go")
	run("git", "commit", "-m", "update util")

	// Commit 4: modify both main.go and util.go together — establishes a
	// co-change relationship that git diff-tree can detect (non-root commit).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n\n// sync with util\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "util.go"),
		[]byte("package main\n\nfunc helper() {}\n\n// sync with main\n"), 0644))
	run("git", "add", "main.go", "util.go")
	run("git", "commit", "-m", "sync main and util")

	return dir
}

// ---- Why (blame) ------------------------------------------------------------

func TestWhy_EntireFile(t *testing.T) {
	dir := setupTestRepo(t)

	entries, err := Why(dir, "main.go", 0, 0)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "expected at least one blame entry")

	for _, e := range entries {
		assert.NotEmpty(t, e.SHA, "SHA should be non-empty")
		assert.NotEmpty(t, e.Author, "Author should be non-empty")
		assert.NotEmpty(t, e.Summary, "Summary should be non-empty")
	}
}

func TestWhy_LineRange(t *testing.T) {
	dir := setupTestRepo(t)

	entries, err := Why(dir, "main.go", 1, 1)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "expected at least one blame entry for line 1")

	// Every entry must have a non-empty Lines field.
	for _, e := range entries {
		assert.NotEmpty(t, e.Lines)
	}
}

func TestWhy_InvalidFile(t *testing.T) {
	dir := setupTestRepo(t)

	_, err := Why(dir, "nonexistent.go", 0, 0)
	assert.Error(t, err)
}

// ---- Changes ----------------------------------------------------------------

func TestChanges_DefaultSince(t *testing.T) {
	dir := setupTestRepo(t)

	// Use a very wide window to ensure commits made in the test are included.
	summaries, err := Changes(dir, "1y", "")
	require.NoError(t, err)
	require.NotEmpty(t, summaries, "expected at least one change summary")

	for _, s := range summaries {
		assert.NotEmpty(t, s.SHA, "SHA should be non-empty")
		assert.NotEmpty(t, s.Author, "Author should be non-empty")
		assert.NotEmpty(t, s.Subject, "Subject should be non-empty")
		assert.NotEmpty(t, s.Files, "Files should be non-empty")
	}
}

func TestChanges_WithPath(t *testing.T) {
	dir := setupTestRepo(t)

	summaries, err := Changes(dir, "1y", "main.go")
	require.NoError(t, err)
	require.NotEmpty(t, summaries)

	// Every returned commit must include main.go in its file list.
	for _, s := range summaries {
		found := false
		for _, f := range s.Files {
			if f.File == "main.go" {
				found = true
				break
			}
		}
		assert.True(t, found, "commit %s should touch main.go, got files: %v", s.SHA, s.Files)
	}
}

// ---- Owners -----------------------------------------------------------------

func TestOwners(t *testing.T) {
	dir := setupTestRepo(t)

	entries, err := Owners(dir, "", 0)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	// All four commits were authored by "Test".
	found := false
	for _, e := range entries {
		if e.Author == "Test" {
			assert.GreaterOrEqual(t, e.Commits, 4, "Test author should have at least 4 commits")
			found = true
			break
		}
	}
	assert.True(t, found, "expected author 'Test' in owners list")
}

func TestOwners_WithLimit(t *testing.T) {
	dir := setupTestRepo(t)

	entries, err := Owners(dir, "", 1)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "limit=1 should return exactly 1 entry")
}

// ---- CoChange ---------------------------------------------------------------

func TestCoChange(t *testing.T) {
	dir := setupTestRepo(t)

	// main.go and util.go were committed together in commit 1.
	entries, err := CoChange(dir, "main.go", 0)
	require.NoError(t, err)

	// util.go appeared in the same commit as main.go, so it should be present.
	found := false
	for _, e := range entries {
		if e.File == "util.go" {
			found = true
			assert.GreaterOrEqual(t, e.Count, 1)
			break
		}
	}
	assert.True(t, found, "expected util.go in co-change entries for main.go")
}

func TestCoChange_EmptyFile(t *testing.T) {
	dir := setupTestRepo(t)

	_, err := CoChange(dir, "", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file is required")
}

// ---- Log --------------------------------------------------------------------

func TestLog(t *testing.T) {
	dir := setupTestRepo(t)

	out, err := Log(dir, 1024, "")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// All three commit subjects should appear in the output.
	assert.Contains(t, out, "initial commit")
	assert.Contains(t, out, "update main")
	assert.Contains(t, out, "update util")
}

func TestLog_WithPath(t *testing.T) {
	dir := setupTestRepo(t)

	out, err := Log(dir, 1024, "main.go")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// Commits touching main.go should appear.
	assert.Contains(t, out, "initial commit")
	assert.Contains(t, out, "update main")
}

// ---- normalizeSince ---------------------------------------------------------

func TestNormalizeSince(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"7d", "7 days ago"},
		{"2w", "2 weeks ago"},
		{"1m", "1 months ago"},
		{"1y", "1 years ago"},
		{"24h", "24 hours ago"},
		{"arbitrary", "arbitrary"},
		{"", "7 days ago"},
		{"2025-01-01", "2025-01-01"},
		{"30d", "30 days ago"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := normalizeSince(tc.input)
			assert.Equal(t, tc.expected, got)
		})
	}
}
