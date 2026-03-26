package command

import (
	"fmt"
	"io"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

// RunUpdate updates extensions from the CLI (no ext.App needed).
func RunUpdate(w io.Writer, settings config.Settings) error {
	fmt.Fprintln(w, "Updating extensions...")
	return InstallOfficialExtensions(w, settings)
}

func registerUpdate(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "update",
		Description: "Update extensions to latest",
		Handler: func(args string, a *ext.App) error {
			settings, err := config.Load()
			if err != nil {
				a.ShowMessage("Failed to load config: " + err.Error())
				return nil
			}
			var b strings.Builder
			b.WriteString("Updating extensions...\n")
			if err := InstallOfficialExtensions(&b, settings); err != nil {
				a.ShowMessage("Update failed: " + err.Error())
				return nil
			}
			b.WriteString("\nExtensions updated. Restart piglet to reload.")
			a.ShowMessage(b.String())
			return nil
		},
	})
}
