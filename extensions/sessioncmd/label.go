package sessioncmd

import (
	"context"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

func registerLabel(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "label",
		Description: "Bookmark the current session node with a label (/label <text> | --clear | (no args))",
		Handler: func(ctx context.Context, args string) error {
			nodes, err := e.SessionFullTree(ctx)
			if err != nil {
				e.ShowMessage("Failed to load session tree: " + err.Error())
				return nil
			}
			id, ok := currentLeafID(nodes)
			if !ok {
				e.ShowMessage("No active session node")
				return nil
			}
			short := id
			if len(short) > 8 {
				short = short[:8]
			}

			trimmed := strings.TrimSpace(args)

			switch {
			case trimmed == "--clear":
				if err := e.SetLabel(ctx, id, ""); err != nil {
					e.ShowMessage("Failed to clear label: " + err.Error())
					return nil
				}
				e.ShowMessage("Cleared label on " + short)

			case trimmed == "":
				// Query mode: find the label on the current leaf.
				var current string
				for _, n := range nodes {
					if n.ID == id {
						current = n.Label
						break
					}
				}
				if current == "" {
					e.ShowMessage(short + ": (no label)")
				} else {
					e.ShowMessage(short + ": " + current)
				}

			default:
				if err := e.SetLabel(ctx, id, trimmed); err != nil {
					e.ShowMessage("Failed to set label: " + err.Error())
					return nil
				}
				e.ShowMessage("Labeled " + short + ": " + trimmed)
			}

			return nil
		},
	})
}

// currentLeafID returns the ID of the deepest node on the active path.
// Returns ("", false) when no active-path node exists.
func currentLeafID(nodes []sdk.TreeNode) (string, bool) {
	bestID := ""
	bestDepth := -1
	for _, n := range nodes {
		if n.OnActivePath && n.Depth > bestDepth {
			bestDepth = n.Depth
			bestID = n.ID
		}
	}
	if bestID == "" {
		return "", false
	}
	return bestID, true
}
