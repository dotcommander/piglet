package prompt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterProjectDocs_SingleFile(t *testing.T) {
	t.Parallel()

	// Create a fake git repo root with a doc file.
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(repoRoot, "CLAUDE.md"),
		[]byte("# Project Instructions\n\nBe helpful."),
		0o644,
	))

	app := ext.NewApp(repoRoot)
	docs := []config.ProjectDoc{
		{Name: "CLAUDE.md", Title: "Project Instructions"},
	}
	prompt.RegisterProjectDocs(app, docs)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	assert.Equal(t, "Project Instructions", sections[0].Title)
	assert.Equal(t, 30, sections[0].Order)
	assert.Contains(t, sections[0].Content, "Be helpful.")
}

func TestRegisterProjectDocs_MultipleFiles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte("claude content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "agents.md"), []byte("agents content"), 0o644))

	app := ext.NewApp(repoRoot)
	docs := []config.ProjectDoc{
		{Name: "CLAUDE.md", Title: "Project Instructions"},
		{Name: "agents.md", Title: "Agents"},
	}
	prompt.RegisterProjectDocs(app, docs)

	sections := app.PromptSections()
	require.Len(t, sections, 2)

	titles := make([]string, len(sections))
	for i, s := range sections {
		titles[i] = s.Title
	}
	assert.Contains(t, titles, "Project Instructions")
	assert.Contains(t, titles, "Agents")
}

func TestRegisterProjectDocs_MissingFilesSkipped(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))
	// Only write one of two files
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte("present"), 0o644))

	app := ext.NewApp(repoRoot)
	docs := []config.ProjectDoc{
		{Name: "CLAUDE.md", Title: "Project Instructions"},
		{Name: "nonexistent.md", Title: "Missing"},
	}
	prompt.RegisterProjectDocs(app, docs)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	assert.Equal(t, "Project Instructions", sections[0].Title)
}

func TestRegisterProjectDocs_EmptyFileSkipped(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte("   \n  "), 0o644))

	app := ext.NewApp(repoRoot)
	docs := []config.ProjectDoc{
		{Name: "CLAUDE.md", Title: "Project Instructions"},
	}
	prompt.RegisterProjectDocs(app, docs)

	assert.Empty(t, app.PromptSections())
}

func TestRegisterProjectDocs_NilDocsIsNoOp(t *testing.T) {
	t.Parallel()

	// Nil docs → no-op; files in the repo root are not read.
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte("instructions"), 0o644))

	app := ext.NewApp(repoRoot)
	prompt.RegisterProjectDocs(app, nil)

	// Nil slice is a no-op — callers supply the doc list (via applyDefaults).
	assert.Empty(t, app.PromptSections())
}

func TestRegisterProjectDocs_EmptyDocsIsNoOp(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte("instructions"), 0o644))

	app := ext.NewApp(repoRoot)
	prompt.RegisterProjectDocs(app, []config.ProjectDoc{})

	assert.Empty(t, app.PromptSections())
}

func TestRegisterProjectDocs_NoCWD_UsesGivenDir(t *testing.T) {
	t.Parallel()

	// If CWD is not a git repo, falls back to CWD itself.
	repoRoot := t.TempDir() // no .git
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte("fallback content"), 0o644))

	app := ext.NewApp(repoRoot)
	docs := []config.ProjectDoc{
		{Name: "CLAUDE.md", Title: "Fallback"},
	}
	prompt.RegisterProjectDocs(app, docs)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	assert.Equal(t, "Fallback", sections[0].Title)
	assert.Contains(t, sections[0].Content, "fallback content")
}

func TestRegisterProjectDocs_FindsRepoRootAboveCWD(t *testing.T) {
	t.Parallel()

	// .git is two levels up — the function should walk up.
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))
	subDir := filepath.Join(repoRoot, "a", "b")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte("root doc"), 0o644))

	app := ext.NewApp(subDir)
	docs := []config.ProjectDoc{
		{Name: "CLAUDE.md", Title: "Root Doc"},
	}
	prompt.RegisterProjectDocs(app, docs)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	assert.Contains(t, sections[0].Content, "root doc")
}
