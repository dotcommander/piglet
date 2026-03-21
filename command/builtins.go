// Package command registers piglet's built-in slash commands via the extension API.
package command

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/modelsdev"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
)

// RegisterBuiltins registers all built-in slash commands and keyboard shortcuts.
func RegisterBuiltins(app *ext.App, models []core.Model, sessDir string, registry *provider.Registry, auth *config.Auth) {
	registerHelp(app)
	registerClear(app)
	registerStep(app)
	registerCompact(app)
	registerExport(app)
	registerModel(app, models)
	registerSession(app, sessDir)
	registerModelsSync(app, registry, auth)
	registerQuit(app)

	// Keyboard shortcuts — delegate to the corresponding commands
	app.RegisterShortcut(&ext.Shortcut{
		Key:         "ctrl+p",
		Description: "Open model selector",
		Handler: func(a *ext.App) error {
			cmds := a.Commands()
			if cmd, ok := cmds["model"]; ok {
				return cmd.Handler("", a)
			}
			return nil
		},
	})
	app.RegisterShortcut(&ext.Shortcut{
		Key:         "ctrl+s",
		Description: "Open session picker",
		Handler: func(a *ext.App) error {
			cmds := a.Commands()
			if cmd, ok := cmds["session"]; ok {
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

			// Sort by name for consistent output
			names := make([]string, 0, len(cmds))
			for name := range cmds {
				names = append(names, name)
			}
			// Simple sort
			for i := range names {
				for j := i + 1; j < len(names); j++ {
					if names[i] > names[j] {
						names[i], names[j] = names[j], names[i]
					}
				}
			}

			for _, name := range names {
				cmd := cmds[name]
				b.WriteString(fmt.Sprintf("  /%-10s — %s\n", name, cmd.Description))
			}

			b.WriteString("\nShortcuts:\n")
			b.WriteString("  ctrl+c    — stop agent / quit\n")
			b.WriteString("  ctrl+p    — model selector\n")
			b.WriteString("  ctrl+s    — session picker\n")

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
			compacted := session.Compact(msgs, session.CompactOptions{KeepRecent: 6})
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

func registerModel(app *ext.App, models []core.Model) {
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
						a.SetModel(mod)
						a.SetStatus("model", mod.Name)
						a.ShowMessage("Switched to " + mod.Name)
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
			items := make([]ext.PickerItem, len(summaries))
			for i, s := range summaries {
				label := s.Title
				if label == "" {
					label = s.ID[:8]
				}
				items[i] = ext.PickerItem{
					ID:    s.Path,
					Label: label,
					Desc:  s.CreatedAt.Format("2006-01-02 15:04"),
				}
			}
			a.ShowPicker("Select Session", items, func(selected ext.PickerItem) {
				sess, err := session.Open(selected.ID)
				if err != nil {
					a.ShowMessage("Failed to open session: " + err.Error())
					return
				}
				msgs := sess.Messages()
				a.SetConversationMessages(msgs)
				a.ShowMessage("Loaded session: " + selected.Label)
				// Note: session file handle management is handled by the TUI
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
