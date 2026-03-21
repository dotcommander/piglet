package memory

import (
	"strings"

	"github.com/dotcommander/piglet/ext"
)

const promptContentCap = 8000

func registerPromptSection(app *ext.App, store *Store) {
	content := buildMemoryPrompt(store)
	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Project Memory",
		Content: content,
		Order:   50,
	})
}

func buildMemoryPrompt(store *Store) string {
	var b strings.Builder

	b.WriteString("You have persistent project memory that survives across sessions. USE IT PROACTIVELY:\n")
	b.WriteString("- When the user tells you something important about the project, their preferences, or asks you to remember something → call memory_set immediately\n")
	b.WriteString("- When you discover project patterns, conventions, or key facts → call memory_set\n")
	b.WriteString("- When you need context from previous sessions → call memory_list or memory_get\n")
	b.WriteString("- Memory persists to disk — anything saved here will be available in future sessions\n\n")

	facts := store.List("")
	if len(facts) == 0 {
		b.WriteString("No memories stored yet. Start saving important facts!")
		return b.String()
	}

	b.WriteString("Current memories:\n")

	lines := make([]string, len(facts))
	total := 0
	for i, f := range facts {
		if f.Category != "" {
			lines[i] = "- " + f.Key + ": " + f.Value + " (" + f.Category + ")"
		} else {
			lines[i] = "- " + f.Key + ": " + f.Value
		}
		total += len(lines[i]) + 1
	}

	// Trim oldest entries to fit cap
	start := 0
	for total > promptContentCap && start < len(lines) {
		total -= len(lines[start]) + 1
		start++
	}

	for _, l := range lines[start:] {
		b.WriteString(l)
		b.WriteByte('\n')
	}

	return b.String()
}
