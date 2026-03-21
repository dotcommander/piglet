package skill

import (
	"strings"

	"github.com/dotcommander/piglet/ext"
)

const promptOrder = 25 // after selfknowledge (20), before projectdocs (30)

func registerPromptSection(app *ext.App, store *Store) {
	skills := store.List()
	if len(skills) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("Available skills (call skill_load to use):\n")
	for _, sk := range skills {
		b.WriteString("- ")
		b.WriteString(sk.Name)
		if sk.Description != "" {
			b.WriteString(": ")
			b.WriteString(sk.Description)
		}
		b.WriteByte('\n')
	}

	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Skills",
		Content: b.String(),
		Order:   promptOrder,
	})
}
