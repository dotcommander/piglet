package command

import (
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

// registerMouse adds the /mouse slash command to toggle TUI mouse capture.
// The toggle flips cfg.MouseCapture, persists to disk, and enqueues an
// ActionSetMouseCapture for the TUI to apply on its next render.
func registerMouse(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "mouse",
		Description: "Toggle mouse capture (wheel scroll vs native text selection)",
		Handler: func(args string, a *ext.App) error {
			settings, err := config.Load()
			if err != nil {
				a.ShowMessage("load config: " + err.Error())
				return nil
			}
			// Toggle. nil (default) is treated as true.
			next := !settings.MouseCaptureEnabled()
			settings.MouseCapture = &next
			if err := config.Save(settings); err != nil {
				a.ShowMessage("save config: " + err.Error())
				return nil
			}
			a.EnqueueAction(ext.ActionSetMouseCapture{Enabled: next})
			return nil
		},
	})
}
