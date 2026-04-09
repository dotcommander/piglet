// Package prompt builds the system prompt from tools, config, and extensions.
package prompt

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dotcommander/piglet/ext"
)

// BuildOptions controls optional prompt building behavior.
// Pass nil to use defaults (no overrides, full tool mode).
type BuildOptions struct {
	OrderOverrides map[string]int // section title → order override
	ToolMode       ext.ToolMode   // controls how deferred tools appear in prompt
}

// Build constructs the system prompt from:
// 1. Base identity string (fallback if no user prompt file)
// 2. User prompt file (~/.config/piglet/prompt.md) — overrides base
// 3. Extension-registered prompt sections
// 4. Tool hints and guidelines from registered tools
func Build(app *ext.App, base string, opts *BuildOptions) string {
	var b strings.Builder

	// User prompt file overrides the base identity
	if userPrompt := loadUserPrompt(); userPrompt != "" {
		b.WriteString(userPrompt)
		b.WriteString("\n\n")
	} else if base != "" {
		b.WriteString(base)
		b.WriteString("\n\n")
	}

	// Extension-registered prompt sections (pre-sorted by Order; re-sorted only when overrides apply)
	sections := app.PromptSections()
	if opts != nil && len(opts.OrderOverrides) > 0 {
		for i, s := range sections {
			if order, ok := opts.OrderOverrides[s.Title]; ok {
				sections[i].Order = order
			}
		}
		slices.SortStableFunc(sections, func(a, b ext.PromptSection) int {
			return cmp.Compare(a.Order, b.Order)
		})
	}
	for _, section := range sections {
		if section.Title != "" {
			b.WriteString("# ")
			b.WriteString(section.Title)
			b.WriteString("\n\n")
		}
		b.WriteString(section.Content)
		b.WriteString("\n\n")
	}

	// Tool hints and guidelines (descriptions already sent via API tool schemas)
	compact := opts != nil && opts.ToolMode == ext.ToolModeCompact
	notes, deferred := buildToolNotes(app.ToolDefs(), compact)
	if notes != "" {
		b.WriteString("# Tool Usage Notes\n\n")
		b.WriteString(notes)
	}
	if deferred != "" {
		b.WriteString("# Available Tools\n\n")
		b.WriteString("These tools are available but their parameter schemas are omitted to save context. Use `tool_search` to look up a tool's complete schema before calling it.\n\n")
		b.WriteString(deferred)
		b.WriteString("\n")
	}

	return b.String()
}

// buildToolNotes extracts tool usage notes and deferred tool index from the
// given tool definitions. Returns two strings: hints/guides and deferred index.
func buildToolNotes(defs []*ext.ToolDef, compact bool) (notes, deferred string) {
	var toolNotes strings.Builder
	var deferredIndex strings.Builder
	for _, td := range defs {
		if td.Deferred && compact {
			deferredIndex.WriteString("- **")
			deferredIndex.WriteString(td.Name)
			deferredIndex.WriteString("**: ")
			deferredIndex.WriteString(td.Description)
			deferredIndex.WriteString("\n")
			continue
		}
		if td.PromptHint == "" && len(td.PromptGuides) == 0 {
			continue
		}
		toolNotes.WriteString("## ")
		toolNotes.WriteString(td.Name)
		if td.PromptHint != "" {
			toolNotes.WriteString(" — ")
			toolNotes.WriteString(td.PromptHint)
		}
		toolNotes.WriteString("\n")
		for _, guide := range td.PromptGuides {
			toolNotes.WriteString("- ")
			toolNotes.WriteString(guide)
			toolNotes.WriteString("\n")
		}
		toolNotes.WriteString("\n")
	}
	return toolNotes.String(), deferredIndex.String()
}

// loadUserPrompt reads ~/.config/piglet/prompt.md if it exists.
// Uses direct file read — prompt.md is a top-level user config file, not an extension config.
func loadUserPrompt() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "piglet", "prompt.md"))
	if err != nil {
		return ""
	}
	return string(data)
}
