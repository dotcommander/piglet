package sessioncmd

import (
	"context"
	"fmt"
	"time"

	"github.com/dotcommander/piglet/sdk"
)

func registerBranch(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "branch",
		Description: "Branch to an earlier point in this session",
		Handler: func(ctx context.Context, args string) error {
			infos, err := e.SessionEntryInfos(ctx)
			if err != nil {
				e.ShowMessage("Failed to list entries: " + err.Error())
				return nil
			}
			if len(infos) == 0 {
				e.ShowMessage("No entries in current session")
				return nil
			}
			items := make([]sdk.PickerItem, len(infos))
			for i, info := range infos {
				idTail := info.ID
				if len(idTail) > 8 {
					idTail = idTail[:8]
				}
				label := fmt.Sprintf("[%s] %s", info.Type, idTail)
				desc := formatShortTimestamp(info.Timestamp)
				if info.Children > 1 {
					desc += fmt.Sprintf(" (%d branches)", info.Children)
				}
				items[i] = sdk.PickerItem{ID: info.ID, Label: label, Desc: desc}
			}
			selected, err := e.ShowPicker(ctx, "Branch to entry", items)
			if err != nil || selected == "" {
				return nil
			}
			if err := e.BranchSession(ctx, selected); err != nil {
				e.ShowMessage("Branch failed: " + err.Error())
				return nil
			}
			e.ShowMessage("Branched to " + selected)
			return nil
		},
	})
}

// formatShortTimestamp parses an RFC3339 timestamp and returns HH:MM:SS.
// On parse failure returns the original string.
func formatShortTimestamp(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("15:04:05")
}
