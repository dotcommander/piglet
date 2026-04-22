// Package cmdcore registers the five core slash commands as an external extension.
// Mirrors command/builtins.go but operates through the SDK's RPC bridge.
package cmdcore

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

// Register registers the five core commands: /help, /clear, /step, /compact, /quit.
func Register(e *sdk.Extension) {
	registerHelp(e)
	registerClear(e)
	registerStep(e)
	registerCompact(e)
	registerQuit(e)
}

func registerHelp(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "help",
		Description: "Show available commands",
		Handler: func(ctx context.Context, args string) error {
			cmds, err := e.Commands(ctx)
			if err != nil {
				e.ShowMessage("help: could not fetch commands: " + err.Error())
				return nil
			}
			shortcuts, err := e.Shortcuts(ctx)
			if err != nil {
				e.ShowMessage("help: could not fetch shortcuts: " + err.Error())
				return nil
			}

			var b strings.Builder
			b.WriteString("Available commands:\n")

			names := make([]string, len(cmds))
			for i, c := range cmds {
				names[i] = c.Name
			}
			slices.Sort(names)

			// Build a lookup for descriptions.
			descOf := make(map[string]string, len(cmds))
			for _, c := range cmds {
				descOf[c.Name] = c.Description
			}

			for _, name := range names {
				fmt.Fprintf(&b, "  /%-10s — %s\n", name, descOf[name])
			}

			b.WriteString("\nShortcuts:\n")
			b.WriteString("  ctrl+c    — stop agent / quit\n")

			// Append registered shortcuts sorted by key.
			keys := make([]string, 0, len(shortcuts))
			for k := range shortcuts {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "  %-10s — %s\n", k, shortcuts[k].Description)
			}

			b.WriteString("\nExtensions: /extensions to see loaded extensions\n")
			e.ShowMessage(b.String())
			return nil
		},
	})
}

func registerClear(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "clear",
		Description: "Clear conversation",
		Handler: func(ctx context.Context, _ string) error {
			// nil clears the conversation history; TUI clears its own display
			// state independently before this handler runs.
			if err := e.SetConversationMessages(ctx, nil); err != nil {
				e.ShowMessage("clear: " + err.Error())
			}
			return nil
		},
	})
}

func registerStep(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "step",
		Description: "Toggle step-by-step tool approval",
		Handler: func(ctx context.Context, _ string) error {
			on, err := e.ToggleStepMode(ctx)
			if err != nil {
				e.ShowMessage("step: " + err.Error())
				return nil
			}
			state := "off"
			if on {
				state = "on"
			}
			e.ShowMessage("Step mode: " + state)
			return nil
		},
	})
}

func registerCompact(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "compact",
		Description: "Compact conversation history",
		Handler: func(ctx context.Context, _ string) error {
			present, err := e.HasCompactor(ctx)
			if err != nil {
				e.ShowMessage("compact: " + err.Error())
				return nil
			}
			if !present {
				e.ShowMessage("No compactor registered")
				return nil
			}
			result, err := e.TriggerCompact(ctx)
			if err != nil {
				e.ShowMessage("compact: " + err.Error())
				return nil
			}
			e.ShowMessage(fmt.Sprintf("Compacted %d → %d messages", result.Before, result.After))
			return nil
		},
	})
}

func registerQuit(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "quit",
		Description: "Exit piglet",
		Handler: func(ctx context.Context, _ string) error {
			if err := e.RequestQuit(ctx); err != nil {
				e.ShowMessage("quit: " + err.Error())
				return nil
			}
			e.ShowMessage("Quitting...")
			return nil
		},
	})
}
