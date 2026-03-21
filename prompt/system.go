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
	} else if base != "" {
		b.WriteString(base)
	}
	b.WriteString("\n\n")

	// Extension-registered prompt sections
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

	// Tool hints and guidelines
	defs := app.ToolDefs()
	if len(defs) > 0 {
		b.WriteString("# Tools\n\n")
		for _, td := range defs {
			b.WriteString("## ")
			b.WriteString(td.Name)
			if td.PromptHint != "" {
				b.WriteString(" — ")
				b.WriteString(td.PromptHint)
			}
			b.WriteString("\n")

			if td.Description != "" {
				b.WriteString(td.Description)
				b.WriteString("\n")
			}

			for _, guide := range td.PromptGuides {
				b.WriteString("- ")
				b.WriteString(guide)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// loadUserPrompt reads ~/.config/piglet/prompt.md if it exists.
func loadUserPrompt() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "piglet", "prompt.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
