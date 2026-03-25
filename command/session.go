package command

import (
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

func sessionPickerItems(summaries []ext.SessionSummary) []ext.PickerItem {
	items := make([]ext.PickerItem, len(summaries))
	for i, s := range summaries {
		label := s.Title
		if label == "" {
			label = s.ID[:8]
		}
		desc := s.CreatedAt.Format("2006-01-02 15:04")
		if s.CWD != "" {
			desc += " — " + s.CWD
		}
		if s.ParentID != "" {
			parentShort := s.ParentID
			if len(parentShort) > 8 {
				parentShort = parentShort[:8]
			}
			desc += " (forked from " + parentShort + ")"
		}
		items[i] = ext.PickerItem{
			ID:    s.Path,
			Label: label,
			Desc:  desc,
		}
	}
	return items
}
