package memory

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/ext"
)

func registerCommand(app *ext.App, store *Store) {
	app.RegisterCommand(&ext.Command{
		Name:        "memory",
		Description: "List, delete, or clear project memories",
		Handler:     makeHandler(store),
		Complete: func(prefix string) []string {
			return []string{"clear", "delete"}
		},
	})
}

func makeHandler(store *Store) func(args string, app *ext.App) error {
	return func(args string, app *ext.App) error {
		args = strings.TrimSpace(args)

		switch {
		case args == "":
			return handleList(store, app)
		case args == "clear":
			if err := store.Clear(); err != nil {
				app.ShowMessage(fmt.Sprintf("error: %s", err))
				return nil
			}
			app.ShowMessage("Project memory cleared.")
		case strings.HasPrefix(args, "delete "):
			key := strings.TrimSpace(strings.TrimPrefix(args, "delete "))
			if err := store.Delete(key); err != nil {
				app.ShowMessage(fmt.Sprintf("error: %s", err))
				return nil
			}
			app.ShowMessage(fmt.Sprintf("Deleted: %s", key))
		default:
			app.ShowMessage(usage())
		}
		return nil
	}
}

func handleList(store *Store, app *ext.App) error {
	facts := store.List("")
	if len(facts) == 0 {
		app.ShowMessage("No project memories stored.")
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Project Memory:\n\n")
	for _, f := range facts {
		if f.Category != "" {
			fmt.Fprintf(&b, "  %s: %s (%s)\n", f.Key, f.Value, f.Category)
		} else {
			fmt.Fprintf(&b, "  %s: %s\n", f.Key, f.Value)
		}
	}
	fmt.Fprintf(&b, "\n%d fact(s) stored.", len(facts))
	app.ShowMessage(b.String())
	return nil
}

func usage() string {
	return `Usage: /memory [clear|delete <key>]
  (no args)     — list all memories
  clear         — delete all memories
  delete <key>  — delete a specific memory`
}
