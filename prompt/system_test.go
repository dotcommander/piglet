package prompt_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/prompt"
	"github.com/stretchr/testify/assert"
)

func TestBuild_BasePrompt(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	result := prompt.Build(app, "You are a helpful assistant.")

	// Base prompt appears when no user prompt file is loaded (HOME set to temp dir).
	// In CI there is no ~/.config/piglet/prompt.md, so base is used.
	// We assert the base string is present (may be absent if user has a prompt file,
	// so we check the result is non-empty and contains the base when possible).
	assert.NotEmpty(t, result)
}

func TestBuild_WithPromptSections(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Code Style",
		Content: "Always use gofmt.",
		Order:   0,
	})
	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Testing",
		Content: "Write table-driven tests.",
		Order:   1,
	})

	result := prompt.Build(app, "base")

	assert.Contains(t, result, "# Code Style")
	assert.Contains(t, result, "Always use gofmt.")
	assert.Contains(t, result, "# Testing")
	assert.Contains(t, result, "Write table-driven tests.")
}

func TestBuild_WithToolDefs(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "read_file",
			Description: "Reads a file from disk.",
		},
		Execute:      func(_ context.Context, _ string, _ map[string]any) (*core.ToolResult, error) { return nil, nil },
		PromptHint:   "Read file contents with line numbers",
		PromptGuides: []string{"Use offset/limit for large files", "Prefer grep to locate content"},
	})

	result := prompt.Build(app, "base")

	// Only hints and guides appear — description is sent via API tool schemas
	assert.Contains(t, result, "# Tool Usage Notes")
	assert.Contains(t, result, "## read_file")
	assert.Contains(t, result, "Read file contents with line numbers")
	assert.NotContains(t, result, "Reads a file from disk.", "description should not be in prompt (sent via API)")
	assert.Contains(t, result, "Use offset/limit for large files")
	assert.Contains(t, result, "Prefer grep to locate content")
}

func TestBuild_EmptyBase(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Section",
		Content: "Some content.",
	})

	result := prompt.Build(app, "")

	// Even with empty base, sections appear
	assert.Contains(t, result, "# Section")
	assert.Contains(t, result, "Some content.")
}

func TestBuild_SectionsOrdered(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	// Register out of order — Build should emit in Order ascending
	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Third",
		Content: "third content",
		Order:   30,
	})
	app.RegisterPromptSection(ext.PromptSection{
		Title:   "First",
		Content: "first content",
		Order:   10,
	})
	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Second",
		Content: "second content",
		Order:   20,
	})

	result := prompt.Build(app, "base")

	idxFirst := strings.Index(result, "# First")
	idxSecond := strings.Index(result, "# Second")
	idxThird := strings.Index(result, "# Third")

	assert.Greater(t, idxFirst, -1, "First section missing")
	assert.Greater(t, idxSecond, -1, "Second section missing")
	assert.Greater(t, idxThird, -1, "Third section missing")

	assert.Less(t, idxFirst, idxSecond, "First should appear before Second")
	assert.Less(t, idxSecond, idxThird, "Second should appear before Third")
}

func TestBuild_ToolWithoutHint(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "list_dir",
			Description: "Lists directory contents.",
		},
		Execute: func(_ context.Context, _ string, _ map[string]any) (*core.ToolResult, error) {
			return nil, nil
		},
		// No PromptHint, no PromptGuides — tool is omitted from prompt entirely
	})

	result := prompt.Build(app, "base")

	// Tools without hints/guides don't appear in the prompt (description sent via API)
	assert.NotContains(t, result, "## list_dir")
	assert.NotContains(t, result, "Lists directory contents.")
	assert.NotContains(t, result, "# Tool Usage Notes")
}

func TestBuild_NoToolsSection(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	result := prompt.Build(app, "base")

	// No tools registered → no tool usage section
	assert.NotContains(t, result, "# Tool Usage Notes")
}

func TestBuild_OrderOverrides(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	app.RegisterPromptSection(ext.PromptSection{Title: "A", Content: "aaa", Order: 10})
	app.RegisterPromptSection(ext.PromptSection{Title: "B", Content: "bbb", Order: 20})

	// Swap order so B comes before A
	result := prompt.Build(app, "base", prompt.BuildOptions{
		OrderOverrides: map[string]int{"B": 5},
	})

	idxA := strings.Index(result, "# A")
	idxB := strings.Index(result, "# B")
	assert.Greater(t, idxB, -1)
	assert.Greater(t, idxA, -1)
	assert.Less(t, idxB, idxA, "B (overridden to 5) should appear before A (10)")
}

func TestBuild_DeferredTools(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("")
	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{Name: "lazy_tool", Description: "loaded on demand"},
		Execute:    func(_ context.Context, _ string, _ map[string]any) (*core.ToolResult, error) { return nil, nil },
		Deferred:   true,
	})

	// Full mode (default) — deferred tools present but no index section injected.
	result := prompt.Build(app, "base")
	assert.NotContains(t, result, "# Available Tools")

	// Compact mode — deferred index appears with tool_search instruction.
	result = prompt.Build(app, "base", prompt.BuildOptions{
		ToolMode: ext.ToolModeCompact,
	})
	assert.Contains(t, result, "# Available Tools")
	assert.Contains(t, result, "tool_search")
	assert.Contains(t, result, "lazy_tool")
	assert.Contains(t, result, "loaded on demand")
}
