package command

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/ext/external"
	"github.com/dotcommander/piglet/tool"
)

func registerExtensions(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "extensions",
		Description: "List loaded extensions, or install/update official extensions",
		Handler: func(args string, a *ext.App) error {
			if sub := strings.TrimSpace(args); sub == "install" || sub == "update" {
				return installExtensions(a)
			}

			infos := a.ExtInfos()

			var b strings.Builder
			if len(infos) == 0 {
				b.WriteString("No extensions loaded.\n")
			} else {
				b.WriteString("Loaded extensions:\n\n")
				for _, info := range infos {
					fmt.Fprintf(&b, "  %s (%s, %s)\n", info.Name, info.Kind, info.Runtime)
					if len(info.Tools) > 0 {
						fmt.Fprintf(&b, "    tools: %s\n", strings.Join(info.Tools, ", "))
					}
					if len(info.Commands) > 0 {
						fmt.Fprintf(&b, "    commands: /%s\n", strings.Join(info.Commands, ", /"))
					}
					b.WriteString("\n")
				}
			}

			extDir, err := external.ExtensionsDir()
			if err == nil {
				fmt.Fprintf(&b, "Extensions dir: %s/\n", extDir)
			}
			b.WriteString("Docs: https://github.com/dotcommander/piglet/blob/main/docs/extensions.md")

			a.ShowMessage(b.String())
			return nil
		},
	})
}

func registerExtInit(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "ext-init",
		Description: "Scaffold a new extension",
		Handler: func(args string, a *ext.App) error {
			name := strings.TrimSpace(args)
			if name == "" {
				a.ShowMessage("Usage: /ext-init <name>\nExample: /ext-init my-tool")
				return nil
			}

			extDir, err := external.ExtensionsDir()
			if err != nil {
				return fmt.Errorf("extensions dir: %w", err)
			}

			dir := filepath.Join(extDir, name)
			if err := os.MkdirAll(dir, 0750); err != nil {
				return fmt.Errorf("create dir: %w", err)
			}

			manifest := fmt.Sprintf(`name: %s
version: 0.1.0
runtime: bun
entry: index.ts
capabilities:
  - tools
  - commands
`, name)

			r := strings.NewReplacer("{{NAME}}", name)
			indexTS := r.Replace(`import { piglet } from "@piglet/sdk";

piglet.setInfo("{{NAME}}", "0.1.0");

piglet.registerTool({
  name: "{{NAME}}_hello",
  description: "A greeting tool",
  parameters: {
    type: "object",
    properties: {
      name: { type: "string", description: "Name to greet" },
    },
    required: ["name"],
  },
  execute: async (args) => {
    return { text: "Hello, " + args.name + "!" };
  },
});

piglet.registerCommand({
  name: "{{NAME}}",
  description: "Run {{NAME}}",
  handler: async (args) => {
    piglet.notify("{{NAME}}: " + (args || "no args"));
  },
});
`)

			if err := os.WriteFile(dir+"/manifest.yaml", []byte(manifest), 0600); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}
			if err := os.WriteFile(dir+"/index.ts", []byte(indexTS), 0600); err != nil {
				return fmt.Errorf("write index.ts: %w", err)
			}

			a.ShowMessage(fmt.Sprintf("Created extension at %s/\n\nFiles:\n  manifest.yaml — extension config\n  index.ts      — your code\n\nInstall SDK: cd %s && bun add @piglet/sdk\nRestart piglet to load.", dir, dir))
			return nil
		},
	})
}

func registerModelsSync(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "models-sync",
		Description: "Sync model catalog from models.dev",
		Handler: func(args string, a *ext.App) error {
			a.ShowMessage("Syncing models from models.dev...")
			updated, err := a.SyncModels()
			if err != nil {
				a.ShowMessage("Sync failed: " + err.Error())
				return nil
			}
			if updated == 0 {
				a.ShowMessage("All models up to date.")
			} else {
				a.ShowMessage(fmt.Sprintf("Sync complete: %d model(s) updated", updated))
			}
			return nil
		},
	})
}

func registerUndo(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "undo",
		Description: "Restore files to pre-edit state",
		Handler: func(args string, a *ext.App) error {
			snapshots, err := tool.UndoSnapshots()
			if err != nil || len(snapshots) == 0 {
				a.ShowMessage("No undo snapshots available")
				return nil
			}

			target := strings.TrimSpace(args)

			// If a specific file is given, restore it directly
			if target != "" {
				for path, data := range snapshots {
					if path == target || strings.HasSuffix(path, "/"+target) {
						if err := os.WriteFile(path, data, 0644); err != nil {
							a.ShowMessage("Undo failed: " + err.Error())
							return nil
						}
						a.ShowMessage("Restored: " + path)
						return nil
					}
				}
				a.ShowMessage("No snapshot for: " + target)
				return nil
			}

			// Show picker with all changed files
			items := make([]ext.PickerItem, 0, len(snapshots))
			for path, data := range snapshots {
				items = append(items, ext.PickerItem{
					ID:    path,
					Label: filepath.Base(path),
					Desc:  fmt.Sprintf("%s (%s)", path, tool.FormatSize(int64(len(data)))),
				})
			}
			// Sort for deterministic order
			slices.SortFunc(items, func(a, b ext.PickerItem) int {
				return strings.Compare(a.ID, b.ID)
			})

			a.ShowPicker("Undo — select file to restore", items, func(selected ext.PickerItem) {
				data, ok := snapshots[selected.ID]
				if !ok {
					a.ShowMessage("Snapshot expired")
					return
				}
				if err := os.WriteFile(selected.ID, data, 0644); err != nil {
					a.ShowMessage("Undo failed: " + err.Error())
					return
				}
				a.ShowMessage("Restored: " + selected.ID)
			})
			return nil
		},
	})
}

func registerConfig(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "config",
		Description: "Show or initialize config",
		Handler: func(args string, a *ext.App) error {
			switch strings.TrimSpace(args) {
			case "--setup":
				a.ShowMessage("Run 'piglet init' from the command line to set up config.")
				return nil

			default:
				dir, _ := config.ConfigDir()
				path, _ := config.SettingsPath()
				authPath, _ := config.AuthPath()
				sessDir, _ := config.SessionsDir()

				var b strings.Builder
				b.WriteString("Config directory: " + dir + "\n")
				b.WriteString("  config.yaml:  ")
				if _, err := os.Stat(path); err == nil {
					b.WriteString(path + "\n")
				} else {
					b.WriteString("(not created — run /config --setup)\n")
				}
				behaviorPath := filepath.Join(dir, "behavior.md")
				b.WriteString("  behavior.md:  ")
				if _, err := os.Stat(behaviorPath); err == nil {
					b.WriteString(behaviorPath + "\n")
				} else {
					b.WriteString("(not created)\n")
				}
				b.WriteString("  auth.json:    ")
				if _, err := os.Stat(authPath); err == nil {
					b.WriteString(authPath + "\n")
				} else {
					b.WriteString("(not created)\n")
				}
				b.WriteString("  sessions/:    ")
				if _, err := os.Stat(sessDir); err == nil {
					b.WriteString(sessDir + "/\n")
				} else {
					b.WriteString("(not created)\n")
				}
				a.ShowMessage(b.String())
				return nil
			}
		},
	})
}
