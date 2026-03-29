package command

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dotcommander/piglet/command/selfupdate"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

// RunUpdate upgrades the piglet binary and rebuilds extensions from the CLI.
func RunUpdate(w io.Writer, settings config.Settings, currentVersion string, opts ...InstallOption) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := selfupdate.CheckAndUpgrade(ctx, w, currentVersion); err != nil {
		fmt.Fprintf(w, "CLI upgrade failed: %v\n", err)
	}
	return InstallOfficialExtensions(w, settings, opts...)
}

func registerUpdate(app *ext.App, version string) {
	app.RegisterCommand(&ext.Command{
		Name:        "update",
		Description: "Upgrade piglet and rebuild extensions",
		Handler: func(args string, a *ext.App) error {
			settings, err := config.Load()
			if err != nil {
				a.ShowMessage("Failed to load config: " + err.Error())
				return nil
			}

			var installOpts []InstallOption
			if strings.Contains(args, "--local") {
				localDir := ""
				parts := strings.Fields(args)
				for i, p := range parts {
					if p == "--local" && i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "-") {
						localDir = parts[i+1]
					}
				}
				if localDir == "" {
					resolved, err := ResolveGoWorkExtPath()
					if err != nil {
						a.ShowMessage("Local source detection failed: " + err.Error())
						return nil
					}
					localDir = resolved
				}
				installOpts = append(installOpts, WithLocalDir(localDir))
			}

			var b strings.Builder
			if err := RunUpdate(&b, settings, version, installOpts...); err != nil {
				a.ShowMessage("Update failed: " + err.Error())
				return nil
			}
			b.WriteString("\nUpdate complete. Restart piglet to reload.")
			a.ShowMessage(b.String())
			return nil
		},
	})
}
