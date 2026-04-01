package command

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/ext"
)

func registerSearch(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "search",
		Description: "Search sessions by title or directory",
		Handler: func(args string, a *ext.App) error {
			query := strings.TrimSpace(args)
			if query == "" {
				a.ShowMessage("Usage: /search <query>")
				return nil
			}
			summaries, err := a.Sessions()
			if err != nil {
				a.ShowMessage("Failed to list sessions: " + err.Error())
				return nil
			}
			lower := strings.ToLower(query)
			var filtered []ext.SessionSummary
			for _, s := range summaries {
				if strings.Contains(strings.ToLower(s.Title), lower) ||
					strings.Contains(strings.ToLower(s.CWD), lower) {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) == 0 {
				a.ShowMessage("No sessions matching: " + query)
				return nil
			}
			items := sessionPickerItems(filtered)
			a.ShowPicker("Search Results", items, func(selected ext.PickerItem) {
				if err := a.LoadSession(selected.ID); err != nil {
					a.ShowMessage("Failed to open session: " + err.Error())
					return
				}
				a.ShowMessage("Loaded session: " + selected.Label)
			})
			return nil
		},
	})
}

func registerBranch(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "branch",
		Description: "Fork session into a new branch",
		Handler:     forkHandler,
	})
}

func registerFork(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "fork",
		Description: "Fork session into a new branch (alias for /branch)",
		Handler:     forkHandler,
	})
}

func forkHandler(args string, a *ext.App) error {
	parentID, count, err := a.ForkSession()
	if err != nil {
		a.ShowMessage("Failed to fork: " + err.Error())
		return nil
	}
	a.ShowMessage(fmt.Sprintf("Forked from %s with %d messages", parentID, count))
	return nil
}

func registerBg(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "bg",
		Description: "Run a prompt in the background",
		Handler: func(args string, a *ext.App) error {
			prompt := strings.TrimSpace(args)
			if prompt == "" {
				a.ShowMessage("Usage: /bg <prompt>")
				return nil
			}
			if a.IsBackgroundRunning() {
				a.ShowMessage("Background agent already running — /bg-cancel to stop it")
				return nil
			}
			if err := a.RunBackground(prompt); err != nil {
				a.ShowMessage("Failed to start background agent: " + err.Error())
				return nil
			}
			a.ShowMessage("Background agent started: " + prompt)
			return nil
		},
	})
}

func registerBgCancel(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "bg-cancel",
		Description: "Cancel the background agent",
		Handler: func(args string, a *ext.App) error {
			if !a.IsBackgroundRunning() {
				a.ShowMessage("No background agent running")
				return nil
			}
			a.CancelBackground()
			a.ShowMessage("Background agent cancelled")
			return nil
		},
	})
}

func registerTitle(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "title",
		Description: "View or set session title",
		Handler: func(args string, a *ext.App) error {
			title := strings.TrimSpace(args)
			if title == "" {
				current := a.SessionTitle()
				if current == "" {
					a.ShowMessage("No title set")
				} else {
					a.ShowMessage("Title: " + current)
				}
				return nil
			}
			if err := a.SetSessionTitle(title); err != nil {
				a.ShowMessage("Failed to set title: " + err.Error())
				return nil
			}
			a.ShowMessage("Title set: " + title)
			return nil
		},
	})
}
