// Package command registers piglet's built-in slash commands via the extension API.
package command

import (
	"context"
	"fmt"
	"maps"
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
	registerModel(app)
	registerSession(app)
	registerSearch(app)
	registerTitle(app)
	registerBranch(app)
	registerBg(app)
	registerBgCancel(app)
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

	for _, sc := range []struct {
		action, desc string
	}{
		{shortcutModel, "Open model selector"},
		{shortcutSession, "Open session picker"},
	} {
		cmdName := sc.action
		app.RegisterShortcut(&ext.Shortcut{
			Key:         keys[sc.action],
			Description: sc.desc,
			Handler: func(a *ext.App) (ext.Action, error) {
				if cmd, ok := a.Commands()[cmdName]; ok {
					return nil, cmd.Handler("", a)
				}
				return nil, nil
			},
		})
	}
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
			compacted := compactMessages(msgs, summary)
			a.SetConversationMessages(compacted)
			a.ShowMessage(fmt.Sprintf("Compacted: %d → %d messages", before, len(compacted)))
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

// compactMessages keeps first message + summary + last 6 messages.
// This is the static fallback when no extension compactor is registered.
func compactMessages(msgs []core.Message, summary string) []core.Message {
	const keepRecent = 6
	if len(msgs) <= keepRecent+1 {
		return msgs
	}
	result := make([]core.Message, 0, keepRecent+2)
	result = append(result, msgs[0])
	result = append(result, &core.AssistantMessage{
		Content:   []core.AssistantContent{core.TextContent{Text: summary}},
		Timestamp: time.Now(),
	})
	result = append(result, msgs[len(msgs)-keepRecent:]...)
	return result
}
