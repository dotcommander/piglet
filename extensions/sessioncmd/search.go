package sessioncmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

func registerSearch(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "search",
		Description: "Search sessions by title or directory",
		Handler: func(ctx context.Context, args string) error {
			query := strings.TrimSpace(args)
			if query == "" {
				e.ShowMessage("Usage: /search <query>")
				return nil
			}
			summaries, err := e.Sessions(ctx)
			if err != nil {
				e.ShowMessage("Failed to list sessions: " + err.Error())
				return nil
			}
			lower := strings.ToLower(query)
			var filtered []sdk.SessionInfo
			for _, s := range summaries {
				if strings.Contains(strings.ToLower(s.Title), lower) ||
					strings.Contains(strings.ToLower(s.CWD), lower) {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) == 0 {
				e.ShowMessage("No sessions matching: " + query)
				return nil
			}
			items := sessionPickerItems(filtered)
			selected, err := e.ShowPicker(ctx, "Search Results", items)
			if err != nil || selected == "" {
				return nil
			}
			if err := e.LoadSession(ctx, selected); err != nil {
				e.ShowMessage("Failed to open session: " + err.Error())
				return nil
			}
			e.ShowMessage("Loaded session: " + selected)
			return nil
		},
	})
}

func registerFork(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "fork",
		Description: "Fork session to a new file",
		Handler: func(ctx context.Context, args string) error {
			parentID, count, err := e.ForkSession(ctx)
			if err != nil {
				e.ShowMessage("Failed to fork: " + err.Error())
				return nil
			}
			e.ShowMessage(fmt.Sprintf("Forked from %s with %d messages", parentID, count))
			return nil
		},
	})
}
