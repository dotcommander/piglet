package prompt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupBehaviorFile creates a piglet config dir in tmp and writes behavior.md with content.
// Returns the XDG_CONFIG_HOME value to set.
func setupBehaviorFile(t *testing.T, content string) string {
	t.Helper()
	tmp := t.TempDir()
	pigletDir := filepath.Join(tmp, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, "behavior.md"), []byte(content), 0o644))
	return tmp
}

// Tests that use t.Setenv cannot use t.Parallel() (Go restriction).
// The env-dependent tests run sequentially; all others run in parallel.

func TestRegisterBehavior_FilePresent(t *testing.T) {
	xdg := setupBehaviorFile(t, "Always be concise.\nNo fluff.")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	app := ext.NewApp(t.TempDir())
	prompt.RegisterBehavior(app)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	assert.Equal(t, "Guidelines", sections[0].Title)
	assert.Equal(t, 10, sections[0].Order)
	assert.Contains(t, sections[0].Content, "Always be concise.")
	assert.Contains(t, sections[0].Content, "No fluff.")
}

func TestRegisterBehavior_FileMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // piglet dir doesn't exist → no behavior.md

	app := ext.NewApp(t.TempDir())
	prompt.RegisterBehavior(app)

	// No file → no section registered
	assert.Empty(t, app.PromptSections())
}

func TestRegisterBehavior_EmptyFile(t *testing.T) {
	xdg := setupBehaviorFile(t, "   \n\t\n  ")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	app := ext.NewApp(t.TempDir())
	prompt.RegisterBehavior(app)

	// Whitespace-only → ReadExtensionConfig trims → empty → no section
	assert.Empty(t, app.PromptSections())
}

func TestRegisterBehavior_OrderBeforeSelfKnowledge(t *testing.T) {
	xdg := setupBehaviorFile(t, "rule1")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	app := ext.NewApp(t.TempDir())
	prompt.RegisterBehavior(app)
	prompt.RegisterSelfKnowledge(app)

	sections := app.PromptSections()
	require.Len(t, sections, 2)

	var behaviorOrder, selfKnowledgeOrder int
	for _, s := range sections {
		switch s.Title {
		case "Guidelines":
			behaviorOrder = s.Order
		case "Current Capabilities":
			selfKnowledgeOrder = s.Order
		}
	}
	assert.Less(t, behaviorOrder, selfKnowledgeOrder, "behavior (order 10) must precede self-knowledge (order 20)")
}
