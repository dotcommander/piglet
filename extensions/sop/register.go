package sop

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// Register wires the /sop command into the shared SDK extension.
//
// On init: creates ~/.config/piglet/sops/plan.md from the embedded default
// if it does not exist, giving users a file they can edit immediately.
//
// /sop <topic> — prepends the resolved SOP to the topic and submits it as a
// user message, kicking off a structured planning session.
// /sop (no args) — prints usage and reports which SOP file is active.
func Register(e *sdk.Extension) {
	var cwd string

	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[sop] OnInit start")

		cwd = x.CWD()

		if err := EnsureGlobalDefault(); err != nil {
			x.Log("warn", fmt.Sprintf("[sop] EnsureGlobalDefault: %v", err))
		}

		x.RegisterCommand(sdk.CommandDef{
			Name:        "sop",
			Description: "Prepend a structured planning SOP to your topic and submit (override at .piglet/sops/plan.md)",
			Handler: func(_ context.Context, args string) error {
				return handleSOP(e, cwd, args)
			},
		})

		x.Log("debug", fmt.Sprintf("[sop] OnInit complete (%s)", time.Since(start)))
	})
}

// handleSOP is the /sop command handler.
// Empty args → print usage + active source.
// Non-empty args → compose SOP+topic message and submit.
func handleSOP(e *sdk.Extension, cwd, args string) error {
	topic := strings.TrimSpace(args)

	content, source, err := Resolve(cwd)
	if err != nil {
		e.Notify(fmt.Sprintf("sop: failed to load SOP: %v", err))
		return nil
	}

	if topic == "" {
		lines := strings.Count(content, "\n") + 1
		e.ShowMessage(fmt.Sprintf("Usage: /sop <topic>\n\nLoaded SOP from %s: %d lines", source, lines))
		return nil
	}

	msg := fmt.Sprintf("%s\n\n---\nTopic: %s\n", strings.TrimRight(content, "\n"), topic)
	e.SendMessage(msg)
	return nil
}
