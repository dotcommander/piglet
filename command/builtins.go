// Package command registers piglet's built-in slash commands via the extension API.
package command

import (
	"context"
	"fmt"

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
	registerMouse(app)
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
