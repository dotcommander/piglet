package prompt_test

import (
	"strings"
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterSelfKnowledge_BasicContent(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	app := ext.NewApp(cwd)
	prompt.RegisterSelfKnowledge(app)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	s := sections[0]

	assert.Equal(t, "Current Capabilities", s.Title)
	assert.Equal(t, 20, s.Order)
	assert.Contains(t, s.Content, "Working directory:")
	assert.Contains(t, s.Content, cwd)
	assert.Contains(t, s.Content, "Platform:")
	assert.Contains(t, s.Content, "Current time:")
}

func TestRegisterSelfKnowledge_WithTools(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{Name: "alpha", Description: "desc"},
	})
	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{Name: "beta", Description: "desc"},
	})

	prompt.RegisterSelfKnowledge(app)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	assert.Contains(t, sections[0].Content, "Registered tools:")
	assert.Contains(t, sections[0].Content, "alpha")
	assert.Contains(t, sections[0].Content, "beta")
}

func TestRegisterSelfKnowledge_WithCommands(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	app.RegisterCommand(&ext.Command{
		Name:    "help",
		Handler: func(_ string, _ *ext.App) error { return nil },
	})
	app.RegisterCommand(&ext.Command{
		Name:    "clear",
		Handler: func(_ string, _ *ext.App) error { return nil },
	})

	prompt.RegisterSelfKnowledge(app)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	content := sections[0].Content
	assert.Contains(t, content, "Slash commands:")
	assert.Contains(t, content, "/clear")
	assert.Contains(t, content, "/help")
}

func TestRegisterSelfKnowledge_CommandsSorted(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	app.RegisterCommand(&ext.Command{
		Name:    "zebra",
		Handler: func(_ string, _ *ext.App) error { return nil },
	})
	app.RegisterCommand(&ext.Command{
		Name:    "apple",
		Handler: func(_ string, _ *ext.App) error { return nil },
	})
	app.RegisterCommand(&ext.Command{
		Name:    "mango",
		Handler: func(_ string, _ *ext.App) error { return nil },
	})

	prompt.RegisterSelfKnowledge(app)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	content := sections[0].Content

	idxApple := strings.Index(content, "apple")
	idxMango := strings.Index(content, "mango")
	idxZebra := strings.Index(content, "zebra")

	assert.Greater(t, idxApple, -1)
	assert.Greater(t, idxMango, -1)
	assert.Greater(t, idxZebra, -1)
	assert.Less(t, idxApple, idxMango, "apple before mango")
	assert.Less(t, idxMango, idxZebra, "mango before zebra")
}

func TestRegisterSelfKnowledge_WithShortcuts(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	app.RegisterShortcut(&ext.Shortcut{
		Key:         "ctrl+v",
		Description: "Paste from clipboard",
	})

	prompt.RegisterSelfKnowledge(app)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	content := sections[0].Content
	assert.Contains(t, content, "Keyboard shortcuts:")
	assert.Contains(t, content, "ctrl+v")
	assert.Contains(t, content, "Paste from clipboard")
}

func TestRegisterSelfKnowledge_NoTools_NoCommands(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	prompt.RegisterSelfKnowledge(app)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	content := sections[0].Content

	// Platform and time always present; tools/commands absent when none registered
	assert.NotContains(t, content, "Registered tools:")
	assert.NotContains(t, content, "Slash commands:")
	assert.NotContains(t, content, "Keyboard shortcuts:")
}

func TestRegisterSelfKnowledge_Order(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	prompt.RegisterSelfKnowledge(app)

	sections := app.PromptSections()
	require.Len(t, sections, 1)
	assert.Equal(t, 20, sections[0].Order)
}
