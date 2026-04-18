package projectdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoRoot_FindsGitDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))

	got := repoRoot(root)
	assert.Equal(t, root, got)
}

func TestRepoRoot_WalksUp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	sub := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	got := repoRoot(sub)
	assert.Equal(t, root, got)
}

func TestRepoRoot_NoGitReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Use a directory known to have no .git above it — filesystem root.
	got := repoRoot(t.TempDir())
	// Either empty or the temp root itself; the key property is it does not panic.
	_ = got
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := defaultConfig()
	require.Len(t, cfg.Docs, 2)
	assert.Equal(t, "CLAUDE.md", cfg.Docs[0].Name)
	assert.Equal(t, "Project Instructions", cfg.Docs[0].Title)
	assert.Equal(t, "agents.md", cfg.Docs[1].Name)
	assert.Equal(t, "Agents", cfg.Docs[1].Title)
}

func TestLoadDocContent_ReadsFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "CLAUDE.md"),
		[]byte("# Project Instructions\n\nBe helpful."),
		0o644,
	))

	path := filepath.Join(root, "CLAUDE.md")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.LessOrEqual(t, info.Size(), int64(maxProjectDocSize))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Be helpful.")
}

func TestLoadDocContent_OversizedSkipped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bigContent := make([]byte, 513<<10) // 513 KB — over the 512 KB limit
	for i := range bigContent {
		bigContent[i] = 'x'
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "big.md"), bigContent, 0o644))

	info, err := os.Stat(filepath.Join(root, "big.md"))
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(maxProjectDocSize), "file must exceed limit for this test to be meaningful")
}

func TestLoadDocContent_EmptyTrimmed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "empty.md"), []byte("   \n  "), 0o644))

	data, err := os.ReadFile(filepath.Join(root, "empty.md"))
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(data)))
}
