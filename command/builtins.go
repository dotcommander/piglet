// Package command registers piglet's built-in slash commands via the extension API.
package command

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
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
func RegisterBuiltins(app *ext.App, shortcuts map[string]string, version string) {
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
	registerUpdate(app)
	registerUpgrade(app, version)
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
