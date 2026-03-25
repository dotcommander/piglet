package command

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/dotcommander/piglet/command/selfupdate"
	"github.com/dotcommander/piglet/ext"
)

// RunUpgrade checks for a new piglet version and installs it from the CLI.
func RunUpgrade(w io.Writer, currentVersion string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return selfupdate.CheckAndUpgrade(ctx, w, currentVersion)
}

func registerUpgrade(app *ext.App, version string) {
	app.RegisterCommand(&ext.Command{
		Name:        "upgrade",
		Description: "Upgrade piglet to latest release",
		Handler: func(args string, a *ext.App) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			var b strings.Builder
			if err := selfupdate.CheckAndUpgrade(ctx, &b, version); err != nil {
				a.ShowMessage("Upgrade failed: " + err.Error())
				return nil
			}
			a.ShowMessage(b.String())
			return nil
		},
	})
}
