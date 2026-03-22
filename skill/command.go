package skill

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/ext"
)

func registerCommand(app *ext.App, store *Store) {
	app.RegisterCommand(&ext.Command{
		Name:        "skill",
		Description: "List or load a skill",
		Handler: func(args string, a *ext.App) error {
			arg := strings.TrimSpace(args)

			// /skill or /skill list — show available skills
			if arg == "" || arg == "list" {
				skills := store.List()
				if len(skills) == 0 {
					a.ShowMessage("No skills found in " + store.Dir())
					return nil
				}
				var b strings.Builder
				b.WriteString("Available skills:\n")
				for _, sk := range skills {
					b.WriteString("  ")
					b.WriteString(sk.Name)
					if sk.Description != "" {
						b.WriteString(" — ")
						b.WriteString(sk.Description)
					}
					b.WriteByte('\n')
				}
				a.ShowMessage(b.String())
				return nil
			}

			// /skill <name> — load skill into conversation
			content, err := store.Load(arg)
			if err != nil {
				a.ShowMessage(fmt.Sprintf("Skill %q not found. Run /skill list to see available skills.", arg))
				return nil
			}
			a.SendMessage(fmt.Sprintf("Load and follow this skill:\n\n%s", content))
			return nil
		},
		Complete: func(prefix string) []string {
			var matches []string
			for _, sk := range store.List() {
				if strings.HasPrefix(sk.Name, prefix) {
					matches = append(matches, sk.Name)
				}
			}
			return matches
		},
	})
}
