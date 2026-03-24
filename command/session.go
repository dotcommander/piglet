package command

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

func registerModel(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "model",
		Description: "Switch model",
		Handler: func(args string, a *ext.App) error {
			models := a.AvailableModels()
			if len(models) == 0 {
				a.ShowMessage("No models available")
				return nil
			}
			items := make([]ext.PickerItem, len(models))
			for i, mod := range models {
				items[i] = ext.PickerItem{
					ID:    mod.Provider + "/" + mod.ID,
					Label: mod.Name,
					Desc:  mod.Provider,
				}
			}
			a.ShowPicker("Select Model", items, func(selected ext.PickerItem) {
				if err := a.SwitchModel(selected.ID); err != nil {
					a.ShowMessage("Failed to switch model: " + err.Error())
					return
				}
				if cfg, err := config.Load(); err == nil {
					cfg.DefaultModel = selected.ID
					if err := config.Save(cfg); err != nil {
						a.ShowMessage("Switched to " + selected.Label + " (failed to save: " + err.Error() + ")")
						return
					}
				}
				a.ShowMessage("Switched to " + selected.Label)
			})
			return nil
		},
	})
}

func registerSession(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "session",
		Description: "Manage sessions",
		Handler: func(args string, a *ext.App) error {
			summaries, err := a.Sessions()
			if err != nil {
				a.ShowMessage(err.Error())
				return nil
			}
			if len(summaries) == 0 {
				a.ShowMessage("No sessions found")
				return nil
			}
			items := sessionPickerItems(summaries)
			a.ShowPicker("Select Session", items, func(selected ext.PickerItem) {
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
				a.ShowMessage(err.Error())
				return nil
			}
			if len(summaries) == 0 {
				a.ShowMessage("No sessions found")
				return nil
			}

			q := strings.ToLower(query)
			var matched []ext.SessionSummary
			for _, s := range summaries {
				if strings.Contains(strings.ToLower(s.Title), q) || strings.Contains(strings.ToLower(s.CWD), q) {
					matched = append(matched, s)
				}
			}
			items := sessionPickerItems(matched)

			if len(items) == 0 {
				a.ShowMessage("No sessions matching: " + query)
				return nil
			}

			a.ShowPicker(fmt.Sprintf("Search: %s (%d results)", query, len(items)), items, func(selected ext.PickerItem) {
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
		Description: "Fork conversation into a new session",
		Handler: func(args string, a *ext.App) error {
			parentID, count, err := a.ForkSession()
			if err != nil {
				a.ShowMessage("Branch failed: " + err.Error())
				return nil
			}
			a.ShowMessage(fmt.Sprintf("Branched from %s (%d messages)", parentID, count))
			return nil
		},
	})
}

func registerTitle(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "title",
		Description: "Set session title",
		Handler: func(args string, a *ext.App) error {
			title := strings.TrimSpace(args)
			if title == "" {
				a.ShowMessage("Usage: /title <title>")
				return nil
			}
			if err := a.SetSessionTitle(title); err != nil {
				a.ShowMessage("Failed to set title: " + err.Error())
				return nil
			}
			a.ShowMessage("Session title: " + title)
			return nil
		},
	})
}

func registerBg(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "bg",
		Description: "Run a read-only background task",
		Handler: func(args string, a *ext.App) error {
			prompt := strings.TrimSpace(args)
			if prompt == "" {
				a.ShowMessage("Usage: /bg <prompt>\nRuns a read-only background agent (max 5 turns).")
				return nil
			}
			if err := a.RunBackground(prompt); err != nil {
				a.ShowMessage("Background task failed: " + err.Error())
				return nil
			}
			a.ShowMessage("Background task started: " + prompt)
			return nil
		},
	})
}

func registerBgCancel(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "bg-cancel",
		Description: "Cancel running background task",
		Handler: func(args string, a *ext.App) error {
			if !a.IsBackgroundRunning() {
				a.ShowMessage("No background task running")
				return nil
			}
			a.CancelBackground()
			a.ShowMessage("Background task cancelled")
			return nil
		},
	})
}
