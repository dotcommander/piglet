package command

import (
	"context"
	"fmt"
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

	fmt.Fprintf(w, "Current version: %s\n", currentVersion)
	fmt.Fprintln(w, "Checking for updates...")

	release, err := selfupdate.FetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("check latest version: %w", err)
	}
	_ = selfupdate.WriteCache(release)

	cmp := selfupdate.CompareVersions(currentVersion, release.TagName)
	if cmp >= 0 {
		fmt.Fprintf(w, "Already up to date (%s)\n", currentVersion)
		return nil
	}

	fmt.Fprintf(w, "Upgrading: %s → %s\n", currentVersion, release.TagName)
	if err := selfupdate.RunUpgrade(ctx, w, release.TagName); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}

	fmt.Fprintf(w, "\nUpgraded to %s. Restart piglet to use the new version.\n", release.TagName)
	return nil
}

func registerUpgrade(app *ext.App, version string) {
	app.RegisterCommand(&ext.Command{
		Name:        "upgrade",
		Description: "Upgrade piglet to latest release",
		Handler: func(args string, a *ext.App) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			a.ShowMessage("Checking for updates...")

			release, err := selfupdate.FetchLatestRelease(ctx)
			if err != nil {
				a.ShowMessage("Check failed: " + err.Error())
				return nil
			}
			_ = selfupdate.WriteCache(release)

			cmp := selfupdate.CompareVersions(version, release.TagName)
			if cmp >= 0 {
				a.ShowMessage(fmt.Sprintf("Already up to date (%s)", version))
				return nil
			}

			a.ShowMessage(fmt.Sprintf("Upgrading: %s → %s ...", version, release.TagName))
			var b strings.Builder
			if err := selfupdate.RunUpgrade(ctx, &b, release.TagName); err != nil {
				a.ShowMessage("Upgrade failed: " + err.Error())
				return nil
			}
			a.ShowMessage(fmt.Sprintf("Upgraded to %s. Restart piglet to use the new version.", release.TagName))
			return nil
		},
	})
}
