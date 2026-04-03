package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dotcommander/piglet/command/selfupdate"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

// RunUpdate upgrades the piglet binary and rebuilds extensions from the CLI.
func RunUpdate(w io.Writer, settings config.Settings, currentVersion string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	upgraded, err := selfupdate.CheckAndUpgrade(ctx, w, currentVersion)
	if err != nil {
		fmt.Fprintf(w, "CLI upgrade failed: %v\n", err)
	}

	// If the binary was replaced, re-exec the NEW binary for extension
	// installation so its code runs instead of this (now stale) process.
	if upgraded {
		return reexecExtensions(w)
	}

	return InstallOfficialExtensions(w, settings)
}

// reexecExtensions runs the newly installed binary with "update --extensions-only"
// so extension installation uses the new code, not the old in-process code.
func reexecExtensions(w io.Writer) error {
	bin, err := exec.LookPath("piglet")
	if err != nil {
		return fmt.Errorf("find new binary: %w", err)
	}
	cmd := exec.Command(bin, "update", "--extensions-only")
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
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

			// Call InstallOfficialExtensions directly — skips the self-upgrade
			// step which may corrupt the terminal's raw mode.
			// Users who want a full binary upgrade run `piglet update` from the CLI.
			var b strings.Builder
			if err := InstallOfficialExtensions(&b, settings); err != nil {
				a.ShowMessage("Update failed: " + err.Error())
				return nil
			}
			b.WriteString("\nUpdate complete. Restart piglet to reload.")
			a.ShowMessage(b.String())
			return nil
		},
	})
}
