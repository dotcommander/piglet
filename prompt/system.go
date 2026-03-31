// Package prompt builds the system prompt from tools, config, and extensions.
package prompt

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

// BuildOptions controls optional prompt building behavior.
type BuildOptions struct {
	OrderOverrides map[string]int // section title → order override
}

// Build constructs the system prompt from:
// 1. Base identity string (fallback if no user prompt file)
// 2. User prompt file (~/.config/piglet/prompt.md) — overrides base
// 3. Extension-registered prompt sections
// 4. Tool hints and guidelines from registered tools
func Build(app *ext.App, base string, opts ...BuildOptions) string {
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
	if len(opts) > 0 && len(opts[0].OrderOverrides) > 0 {
		for i, s := range sections {
			if order, ok := opts[0].OrderOverrides[s.Title]; ok {
				sections[i].Order = order
			}
		}
		slices.SortFunc(sections, func(a, b ext.PromptSection) int {
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
	var toolNotes strings.Builder
	hasDeferred := false
	for _, td := range app.ToolDefs() {
		if td.Deferred {
			hasDeferred = true
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
	if toolNotes.Len() > 0 {
		b.WriteString("# Tool Usage Notes\n\n")
		b.WriteString(toolNotes.String())
	}

	// Deferred tools note
	if hasDeferred {
		b.WriteString("## Deferred Tools\n\n")
		b.WriteString("Some tools are listed by name only — their full parameter schemas are omitted to save context. Use the `tool_search` tool to look up a deferred tool's complete schema before calling it.\n\n")
	}

	return b.String()
}

// loadUserPrompt reads ~/.config/piglet/prompt.md if it exists.
func loadUserPrompt() string {
	s, err := config.ReadExtensionConfig("prompt")
	if err != nil {
		slog.Warn("failed to load user prompt", "error", err)
	}
	return s
}
