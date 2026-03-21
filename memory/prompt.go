package memory

import (
	"strings"

	"github.com/dotcommander/piglet/ext"
)

const promptContentCap = 8000

func registerPromptSection(app *ext.App, store *Store) {
	facts := store.List("")
	if len(facts) == 0 {
		return
	}

	// Build lines and measure total size.
	lines := make([]string, len(facts))
	total := 0
	for i, f := range facts {
		if f.Category != "" {
			lines[i] = "- " + f.Key + ": " + f.Value + " (" + f.Category + ")"
		} else {
			lines[i] = "- " + f.Key + ": " + f.Value
		}
		total += len(lines[i]) + 1 // +1 for newline
	}

	// Trim oldest (front) entries until content fits within cap.
	start := 0
	for total > promptContentCap && start < len(lines) {
		total -= len(lines[start]) + 1
		start++
	}

	if start >= len(lines) {
		return
	}

	var b strings.Builder
	b.Grow(total)
	for _, l := range lines[start:] {
		b.WriteString(l)
		b.WriteByte('\n')
	}

	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Project Memory",
		Content: b.String(),
		Order:   50,
	})
}
