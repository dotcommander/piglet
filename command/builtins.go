// Package command registers piglet's built-in slash commands via the extension API.
package command

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/ext/external"
	"github.com/dotcommander/piglet/tool"
)

// Default keyboard shortcut bindings.
const (
	shortcutModel   = "model"
	shortcutSession = "session"
	keyModel        = "ctrl+p"
	keySession      = "ctrl+s"
)

// RegisterBuiltins registers all built-in slash commands and keyboard shortcuts.
// All commands operate exclusively through the ext.App SDK.
func RegisterBuiltins(app *ext.App, shortcuts map[string]string) {
	registerStatusSections(app)
	registerHelp(app)
	registerClear(app)
	registerStep(app)
	registerCompact(app)
	registerExport(app)
	registerExtensions(app)
	registerExtInit(app)
	registerModel(app)
	registerSession(app)
	registerModelsSync(app)
	registerBranch(app)
	registerBg(app)
	registerBgCancel(app)
	registerSearch(app)
	registerTitle(app)
	registerUndo(app)
	registerConfig(app)
	registerQuit(app)

	// Keyboard shortcuts — delegate to the corresponding commands
	keys := map[string]string{
		shortcutModel:   keyModel,
		shortcutSession: keySession,
	}
	for action, key := range shortcuts {
		keys[action] = key
	}

	app.RegisterShortcut(&ext.Shortcut{
		Key:         keys[shortcutModel],
		Description: "Open model selector",
		Handler: func(a *ext.App) (ext.Action, error) {
			cmds := a.Commands()
			if cmd, ok := cmds[shortcutModel]; ok {
				return nil, cmd.Handler("", a)
			}
			return nil, nil
		},
	})
	app.RegisterShortcut(&ext.Shortcut{
		Key:         keys[shortcutSession],
		Description: "Open session picker",
		Handler: func(a *ext.App) (ext.Action, error) {
			cmds := a.Commands()
			if cmd, ok := cmds[shortcutSession]; ok {
				return nil, cmd.Handler("", a)
			}
			return nil, nil
		},
	})
}

func registerStatusSections(app *ext.App) {
	// Left side
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyApp, Side: ext.StatusLeft, Order: 0})
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyModel, Side: ext.StatusLeft, Order: 10})
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyBg, Side: ext.StatusLeft, Order: 20})

	// Right side
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyTokens, Side: ext.StatusRight, Order: 0})
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyCost, Side: ext.StatusRight, Order: 10})
}

func registerHelp(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "help",
		Description: "Show available commands",
		Handler: func(args string, a *ext.App) error {
			cmds := a.Commands()
			var b strings.Builder
			b.WriteString("Available commands:\n")

			names := slices.Sorted(maps.Keys(cmds))

			for _, name := range names {
				cmd := cmds[name]
				fmt.Fprintf(&b, "  /%-10s — %s\n", name, cmd.Description)
			}

			b.WriteString("\nShortcuts:\n")
			b.WriteString("  ctrl+c    — stop agent / quit\n")
			fmt.Fprintf(&b, "  %-10s — model selector\n", keyModel)
			fmt.Fprintf(&b, "  %-10s — session picker\n", keySession)
			b.WriteString("\nExtensions: /extensions to see loaded extensions\n")

			a.ShowMessage(b.String())
			return nil
		},
	})
}

func registerClear(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "clear",
		Description: "Clear conversation",
		Handler: func(args string, a *ext.App) error {
			// TUI handles clearing messages directly; this is a no-op marker
			return nil
		},
	})
}

func registerStep(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "step",
		Description: "Toggle step-by-step tool approval",
		Handler: func(args string, a *ext.App) error {
			on := a.ToggleStepMode()
			state := "off"
			if on {
				state = "on"
			}
			a.ShowMessage("Step mode: " + state)
			return nil
		},
	})
}

func registerCompact(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "compact",
		Description: "Compact conversation history",
		Handler: func(args string, a *ext.App) error {
			msgs := a.ConversationMessages()
			if len(msgs) < 4 {
				a.ShowMessage("Not enough messages to compact")
				return nil
			}
			before := len(msgs)

			// Use registered compactor if available
			if c := a.Compactor(); c != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				compacted, err := c.Compact(ctx, msgs)
				if err != nil {
					a.ShowMessage("Compact error: " + err.Error())
					return nil
				}
				a.SetConversationMessages(compacted)
				a.ShowMessage(fmt.Sprintf("Compacted: %d → %d messages", before, len(compacted)))
				return nil
			}

			// Fallback: static summary
			summary := fmt.Sprintf("[%d earlier messages compacted]", len(msgs)-7)
			compacted := core.CompactMessages(msgs, summary)
			a.SetConversationMessages(compacted)
			a.ShowMessage(fmt.Sprintf("Compacted: %d → %d messages", before, len(compacted)))
			return nil
		},
	})
}

func registerExport(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "export",
		Description: "Export conversation",
		Handler: func(args string, a *ext.App) error {
			msgs := a.ConversationMessages()
			if len(msgs) == 0 {
				a.ShowMessage("No messages to export")
				return nil
			}
			path := fmt.Sprintf("piglet-export-%s.md", time.Now().Format("20060102-150405"))
			if err := exportMarkdown(msgs, path); err != nil {
				a.ShowMessage("Export failed: " + err.Error())
				return nil
			}
			a.ShowMessage("Exported to " + path)
			return nil
		},
	})
}

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

func registerExtensions(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "extensions",
		Description: "List loaded extensions",
		Handler: func(args string, a *ext.App) error {
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
			if err := os.MkdirAll(dir, 0755); err != nil {
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

			if err := os.WriteFile(dir+"/manifest.yaml", []byte(manifest), 0644); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}
			if err := os.WriteFile(dir+"/index.ts", []byte(indexTS), 0644); err != nil {
				return fmt.Errorf("write index.ts: %w", err)
			}

			a.ShowMessage(fmt.Sprintf("Created extension at %s/\n\nFiles:\n  manifest.yaml — extension config\n  index.ts      — your code\n\nInstall SDK: cd %s && bun add @piglet/sdk\nRestart piglet to load.", dir, dir))
			return nil
		},
	})
}

func registerBranch(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "branch",
		Description: "Fork conversation into a new session",
		Handler: func(args string, a *ext.App) error {
			parentID, count, err := a.ForkSession()
			if err != nil {
				a.ShowMessage("Branch failed: " + err.Error())
				return nil
			}
			a.ShowMessage(fmt.Sprintf("Branched from %s (%d messages)", parentID, count))
			return nil
		},
	})
}

func registerQuit(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "quit",
		Description: "Exit piglet",
		Handler: func(args string, a *ext.App) error {
			a.RequestQuit()
			return nil
		},
	})
}

func registerSearch(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "search",
		Description: "Search sessions by title or directory",
		Handler: func(args string, a *ext.App) error {
			query := strings.TrimSpace(args)
			if query == "" {
				a.ShowMessage("Usage: /search <query>")
				return nil
			}
			summaries, err := a.Sessions()
			if err != nil {
				a.ShowMessage(err.Error())
				return nil
			}
			if len(summaries) == 0 {
				a.ShowMessage("No sessions found")
				return nil
			}

			q := strings.ToLower(query)
			var matched []ext.SessionSummary
			for _, s := range summaries {
				if strings.Contains(strings.ToLower(s.Title), q) || strings.Contains(strings.ToLower(s.CWD), q) {
					matched = append(matched, s)
				}
			}
			items := sessionPickerItems(matched)

			if len(items) == 0 {
				a.ShowMessage("No sessions matching: " + query)
				return nil
			}

			a.ShowPicker(fmt.Sprintf("Search: %s (%d results)", query, len(items)), items, func(selected ext.PickerItem) {
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

func registerTitle(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "title",
		Description: "Set session title",
		Handler: func(args string, a *ext.App) error {
			title := strings.TrimSpace(args)
			if title == "" {
				a.ShowMessage("Usage: /title <title>")
				return nil
			}
			if err := a.SetSessionTitle(title); err != nil {
				a.ShowMessage("Failed to set title: " + err.Error())
				return nil
			}
			a.ShowMessage("Session title: " + title)
			return nil
		},
	})
}

func registerBg(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "bg",
		Description: "Run a read-only background task",
		Handler: func(args string, a *ext.App) error {
			prompt := strings.TrimSpace(args)
			if prompt == "" {
				a.ShowMessage("Usage: /bg <prompt>\nRuns a read-only background agent (max 5 turns).")
				return nil
			}
			if err := a.RunBackground(prompt); err != nil {
				a.ShowMessage("Background task failed: " + err.Error())
				return nil
			}
			a.ShowMessage("Background task started: " + prompt)
			return nil
		},
	})
}

func registerBgCancel(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "bg-cancel",
		Description: "Cancel running background task",
		Handler: func(args string, a *ext.App) error {
			if !a.IsBackgroundRunning() {
				a.ShowMessage("No background task running")
				return nil
			}
			a.CancelBackground()
			a.ShowMessage("Background task cancelled")
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

func exportMarkdown(messages []core.Message, path string) error {
	var b strings.Builder
	b.WriteString("# Piglet Conversation\n\n")

	for _, msg := range messages {
		switch m := msg.(type) {
		case *core.UserMessage:
			b.WriteString("## User\n\n")
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		case *core.AssistantMessage:
			b.WriteString("## Assistant\n\n")
			for _, c := range m.Content {
				switch tc := c.(type) {
				case core.TextContent:
					b.WriteString(tc.Text)
				case core.ThinkingContent:
					b.WriteString("<details><summary>Thinking</summary>\n\n")
					b.WriteString(tc.Thinking)
					b.WriteString("\n</details>")
				}
			}
			b.WriteString("\n\n")
		case *core.ToolResultMessage:
			fmt.Fprintf(&b, "### Tool: %s\n\n", m.ToolName)
			for _, c := range m.Content {
				if tc, ok := c.(core.TextContent); ok {
					b.WriteString(tc.Text)
				}
			}
			b.WriteString("\n\n")
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}
