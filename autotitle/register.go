package autotitle

import (
	"context"
	"sync/atomic"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// Config controls autotitle behavior.
type Config struct {
	Enabled bool // whether auto-title is active (default true)
}

// Register adds the autotitle event handler to the extension app.
// It listens for EventAgentEnd and generates a session title after the first exchange.
func Register(app *ext.App, cfg Config) {
	if !cfg.Enabled {
		return
	}

	prompt := LoadPrompt()
	if prompt == "" {
		return
	}

	var fired atomic.Bool

	app.RegisterEventHandler(ext.EventHandler{
		Name:     "autotitle",
		Priority: 100,
		Filter:   func(e core.Event) bool { _, ok := e.(core.EventAgentEnd); return ok },
		Handle: func(ctx context.Context, _ core.Event) ext.Action {
			if !fired.CompareAndSwap(false, true) {
				return nil
			}

			msgs := app.ConversationMessages()
			if len(msgs) < 2 {
				fired.Store(false) // allow retry on next event
				return nil
			}

			if app.SessionTitle() != "" {
				return nil
			}

			prov := app.Provider()
			if prov == nil {
				return nil
			}

			snapshot := make([]core.Message, len(msgs))
			copy(snapshot, msgs)

			return ext.ActionRunAsync{Fn: func() ext.Action {
				title := GenerateTitle(context.Background(), prov, snapshot, prompt)
				if title != "" {
					return ext.ActionSetSessionTitle{Title: title}
				}
				return nil
			}}
		},
	})
}
