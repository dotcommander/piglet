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

// RegisterBuiltins registers all built-in slash commands.
// Keyboard shortcuts for session (ctrl+s) and model (ctrl+p) live in
// extensions/sessioncmd and are registered there.
// All commands operate exclusively through the ext.App SDK.
//
// The shortcuts parameter is reserved for future use and is currently ignored.
func RegisterBuiltins(app *ext.App, shortcuts map[string]string, version string) {
	_ = shortcuts // reserved for future shortcut customization
	registerStatusSections(app)
	registerPromptBudgetHandler(app)
	registerHelp(app)
	registerClear(app)
	registerStep(app)
	registerCompact(app)
	registerUpdate(app, version)
	app.RegisterCommand(&ext.Command{
		Name:        "upgrade",
		Description: "Alias for /update",
		Handler: func(args string, a *ext.App) error {
			if cmd, ok := a.Commands()["update"]; ok {
				return cmd.Handler(args, a)
			}
			a.ShowMessage("update command not found")
			return nil
		},
	})
	registerQuit(app)
}

func registerStatusSections(app *ext.App) {
	// Left side
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyApp, Side: ext.StatusLeft, Order: 0})
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyModel, Side: ext.StatusLeft, Order: 10})
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyMouse, Side: ext.StatusLeft, Order: 15})
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyBg, Side: ext.StatusLeft, Order: 20})

	// Right side
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyTokens, Side: ext.StatusRight, Order: 0})
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyCost, Side: ext.StatusRight, Order: 10})
	app.RegisterStatusSection(ext.StatusSection{Key: ext.StatusKeyPromptBudget, Side: ext.StatusRight, Order: 20})
}

func registerPromptBudgetHandler(app *ext.App) {
	app.RegisterEventHandler(ext.EventHandler{
		Name:     "prompt-budget",
		Priority: 500,
		Filter: func(evt core.Event) bool {
			_, ok := evt.(core.EventTurnEnd)
			return ok
		},
		Handle: func(_ context.Context, _ core.Event) ext.Action {
			sections := app.PromptSections()
			var total int
			for _, s := range sections {
				total += s.TokenHint
			}
			if total == 0 {
				return ext.ActionSetStatus{Key: ext.StatusKeyPromptBudget, Text: ""}
			}
			return ext.ActionSetStatus{
				Key:  ext.StatusKeyPromptBudget,
				Text: fmt.Sprintf("~%dk ctx", total/1000),
			}
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
				fmt.Fprintf(&b, "  /%-10s — %s\n", name, cmd.Description)
			}

			b.WriteString("\nShortcuts:\n")
			b.WriteString("  ctrl+c    — stop agent / quit\n")
			b.WriteString("  (more shortcuts available via extensions)\n")
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
			// Clear agent history. TUI clears its own display state (m.messages,
			// m.msgCache) in runCommand before this handler runs — those fields are
			// TUI-internal and have no corresponding ext.Action type.
			a.SetConversationMessages(nil)
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
