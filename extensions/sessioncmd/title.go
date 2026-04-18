package sessioncmd

import (
	"context"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

func registerTitle(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "title",
		Description: "View or set session title",
		Handler: func(ctx context.Context, args string) error {
			title := strings.TrimSpace(args)
			if title == "" {
				current, err := e.SessionTitle(ctx)
				if err != nil {
					e.ShowMessage("Failed to read title: " + err.Error())
					return nil
				}
				if current == "" {
					e.ShowMessage("No title set")
				} else {
					e.ShowMessage("Title: " + current)
				}
				return nil
			}
			if err := e.SetSessionTitle(ctx, title); err != nil {
				e.ShowMessage("Failed to set title: " + err.Error())
				return nil
			}
			e.ShowMessage("Title set: " + title)
			return nil
		},
	})
}
