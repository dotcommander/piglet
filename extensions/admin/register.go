package admin

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/sdk"
)

// configFile is a config file entry for display.
type configFile struct {
	label, path string
}

// configFiles returns the known piglet config files for the given directory.
func configFiles(dir string) []configFile {
	return []configFile{
		{"config.yaml", filepath.Join(dir, "config.yaml")},
		{"behavior.md", filepath.Join(dir, "behavior.md")},
		{"auth.json", filepath.Join(dir, "auth.json")},
		{"models.yaml", filepath.Join(dir, "models.yaml")},
		{"sessions/", filepath.Join(dir, "sessions")},
	}
}

// formatFileStatus returns a human-readable status line for a config file.
func formatFileStatus(cf configFile) string {
	info, err := os.Stat(cf.path)
	if err == nil {
		_ = info
		return cf.path
	}
	if os.IsNotExist(err) {
		return "(not created)"
	}
	return "(error: " + err.Error() + ")"
}

type messenger interface {
	ShowMessage(msg string)
}

// runSetup bootstraps the config directory with default files.
func runSetup(e messenger, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.ShowMessage("Cannot create config dir: " + err.Error())
		return
	}
	var created []string
	for _, name := range []string{"config.yaml", "behavior.md"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if f, err := os.Create(p); err == nil {
				f.Close()
				created = append(created, name)
			}
		}
	}
	sessionsDir := filepath.Join(dir, "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(sessionsDir, 0o755); err == nil {
			created = append(created, "sessions/")
		}
	}
	if len(created) == 0 {
		e.ShowMessage("Config already set up at " + dir)
	} else {
		e.ShowMessage("Created: " + strings.Join(created, ", ") + " in " + dir)
	}
}

// Register registers the admin extension's commands.
func Register(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "config",
		Description: "Show config paths and status. Use --setup for initial config.",
		Handler: func(_ context.Context, args string) error {
			trimmed := strings.TrimSpace(args)
			if trimmed == "--setup" {
				dir, err := xdg.ConfigDir()
				if err != nil {
					e.ShowMessage("Cannot determine config dir: " + err.Error())
					return nil
				}
				runSetup(e, dir)
				return nil
			}
			if trimmed != "" {
				e.ShowMessage("Unknown argument: " + trimmed)
				return nil
			}

			dir, err := xdg.ConfigDir()
			if err != nil {
				e.ShowMessage("Cannot determine config dir: " + err.Error())
				return nil
			}

			var b strings.Builder
			b.WriteString("Config directory: " + dir + "\n")
			for _, cf := range configFiles(dir) {
				b.WriteString("  " + cf.label + ":  " + formatFileStatus(cf) + "\n")
			}
			e.ShowMessage(b.String())
			return nil
		},
	})

}
