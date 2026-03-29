package command

import (
	"fmt"
	"io"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

// RunUpgrade is an alias for RunUpdate — upgrades binary and rebuilds extensions.
func RunUpgrade(w io.Writer, currentVersion string) error {
	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	return RunUpdate(w, settings, currentVersion)
}

func registerUpgrade(app *ext.App, version string) {
	app.RegisterCommand(&ext.Command{
		Name:        "upgrade",
		Description: "Alias for /update",
		Handler: func(args string, a *ext.App) error {
			if cmd, ok := a.Commands()["update"]; ok {
				return cmd.Handler(args, a)
			}
			a.ShowMessage("update command not found")
			return nil
		},
	})
}
