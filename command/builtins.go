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
	"github.com/dotcommander/piglet/modelsdev"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
)

// Default keyboard shortcut bindings.
const (
	shortcutModel   = "model"
	shortcutSession = "session"
	keyModel        = "ctrl+p"
	keySession      = "ctrl+s"
)

// RegisterBuiltins registers all built-in slash commands and keyboard shortcuts.
func RegisterBuiltins(app *ext.App, models []core.Model, sessDir string, registry *provider.Registry, auth *config.Auth, settings *config.Settings) {
	registerHelp(app)
	registerClear(app)
	registerStep(app)
	registerCompact(app, settings)
	registerExport(app)
	registerExtensions(app)
	registerExtInit(app)
	registerModel(app, models, registry, auth)
	registerSession(app, sessDir)
	registerModelsSync(app, registry, auth)
	registerBranch(app)
	registerBg(app)
	registerBgCancel(app)
	registerSearch(app, sessDir)
	registerTitle(app)
	registerQuit(app)

	// Keyboard shortcuts — delegate to the corresponding commands
	shortcuts := map[string]string{
		shortcutModel:   keyModel,
		shortcutSession: keySession,
	}
	if settings != nil {
		for action, key := range settings.Shortcuts {
			shortcuts[action] = key
		}
	}

	app.RegisterShortcut(&ext.Shortcut{
		Key:         shortcuts[shortcutModel],
		Description: "Open model selector",
		Handler: func(a *ext.App) error {
			cmds := a.Commands()
			if cmd, ok := cmds[shortcutModel]; ok {
				return cmd.Handler("", a)
			}
			return nil
		},
	})
	app.RegisterShortcut(&ext.Shortcut{
		Key:         shortcuts[shortcutSession],
		Description: "Open session picker",
		Handler: func(a *ext.App) error {
			cmds := a.Commands()
			if cmd, ok := cmds[shortcutSession]; ok {
				return cmd.Handler("", a)
			}
			return nil
		},
	})
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
				b.WriteString(fmt.Sprintf("  /%-10s — %s\n", name, cmd.Description))
			}

			b.WriteString("\nShortcuts:\n")
			b.WriteString("  ctrl+c    — stop agent / quit\n")
			b.WriteString(fmt.Sprintf("  %-10s — model selector\n", keyModel))
			b.WriteString(fmt.Sprintf("  %-10s — session picker\n", keySession))
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

func registerCompact(app *ext.App, settings *config.Settings) {
	keepRecent := 6
	if settings != nil {
		keepRecent = config.IntOr(settings.Agent.CompactKeepRecent, 6)
	}
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
			compacted := session.Compact(msgs, session.CompactOptions{KeepRecent: keepRecent})
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

func registerModel(app *ext.App, models []core.Model, registry *provider.Registry, auth *config.Auth) {
	app.RegisterCommand(&ext.Command{
		Name:        "model",
		Description: "Switch model",
		Handler: func(args string, a *ext.App) error {
			items := make([]ext.PickerItem, len(models))
			for i, mod := range models {
				items[i] = ext.PickerItem{
					ID:    mod.Provider + "/" + mod.ID,
					Label: mod.Name,
					Desc:  mod.Provider,
				}
			}
			a.ShowPicker("Select Model", items, func(selected ext.PickerItem) {
				for _, mod := range models {
					if mod.Provider+"/"+mod.ID == selected.ID {
						apiKeyFn := func() string {
							return auth.GetAPIKey(mod.Provider)
						}
						prov, err := registry.Create(mod, apiKeyFn)
						if err != nil {
							a.ShowMessage("Failed to create provider: " + err.Error())
							return
						}
						a.SetModel(mod)
						a.SetProvider(prov)
						a.SetStatus("model", mod.DisplayName())
						a.ShowMessage("Switched to " + mod.DisplayName())
						break
					}
				}
			})
			return nil
		},
	})
}

func registerSession(app *ext.App, sessDir string) {
	app.RegisterCommand(&ext.Command{
		Name:        "session",
		Description: "Manage sessions",
		Handler: func(args string, a *ext.App) error {
			if sessDir == "" {
				a.ShowMessage("Sessions not configured")
				return nil
			}
			summaries, err := session.List(sessDir)
			if err != nil || len(summaries) == 0 {
				a.ShowMessage("No sessions found")
				return nil
			}
			items := sessionPickerItems(summaries)
			a.ShowPicker("Select Session", items, func(selected ext.PickerItem) {
				sess, err := session.Open(selected.ID)
				if err != nil {
					a.ShowMessage("Failed to open session: " + err.Error())
					return
				}
				a.SwapSession(sess)
				a.ShowMessage("Loaded session: " + selected.Label)
			})
			return nil
		},
	})
}

func registerModelsSync(app *ext.App, registry *provider.Registry, auth *config.Auth) {
	app.RegisterCommand(&ext.Command{
		Name:        "models-sync",
		Description: "Sync model catalog from models.dev",
		Handler: func(args string, a *ext.App) error {
			a.ShowMessage("Syncing models from models.dev...")
			updated, err := modelsdev.Sync(context.Background(), registry, auth)
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
					b.WriteString(fmt.Sprintf("  %s (%s, %s)\n", info.Name, info.Kind, info.Runtime))
					if len(info.Tools) > 0 {
						b.WriteString(fmt.Sprintf("    tools: %s\n", strings.Join(info.Tools, ", ")))
					}
					if len(info.Commands) > 0 {
						b.WriteString(fmt.Sprintf("    commands: /%s\n", strings.Join(info.Commands, ", /")))
					}
					b.WriteString("\n")
				}
			}

			extDir, err := external.ExtensionsDir()
			if err == nil {
				b.WriteString(fmt.Sprintf("Extensions dir: %s/\n", extDir))
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

func registerSearch(app *ext.App, sessDir string) {
	app.RegisterCommand(&ext.Command{
		Name:        "search",
		Description: "Search sessions by title or directory",
		Handler: func(args string, a *ext.App) error {
			query := strings.TrimSpace(args)
			if query == "" {
				a.ShowMessage("Usage: /search <query>")
				return nil
			}
			if sessDir == "" {
				a.ShowMessage("Sessions not configured")
				return nil
			}
			summaries, err := session.List(sessDir)
			if err != nil || len(summaries) == 0 {
				a.ShowMessage("No sessions found")
				return nil
			}

			q := strings.ToLower(query)
			var matched []session.Summary
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
				sess, err := session.Open(selected.ID)
				if err != nil {
					a.ShowMessage("Failed to open session: " + err.Error())
					return
				}
				a.SwapSession(sess)
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

func sessionPickerItems(summaries []session.Summary) []ext.PickerItem {
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
			b.WriteString(fmt.Sprintf("### Tool: %s\n\n", m.ToolName))
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
