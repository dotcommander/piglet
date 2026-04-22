package sessioncmd

import (
	"context"

	"github.com/dotcommander/piglet/sdk"
)

func registerReset(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "reset",
		Description: "Restart from the root user message (edit and resend)",
		Handler: func(ctx context.Context, _ string) error {
			nodes, err := e.SessionFullTree(ctx)
			if err != nil {
				e.ShowMessage("Failed to load session tree: " + err.Error())
				return nil
			}
			rootText := firstActiveUserPreview(nodes)

			if err := e.ResetSessionLeaf(ctx); err != nil {
				e.ShowMessage("Reset failed: " + err.Error())
				return nil
			}

			if rootText != "" {
				e.SetInputText(rootText)
			}
			e.ShowMessage("Session reset — edit and resend, or clear with Esc.")
			return nil
		},
	})
}

// firstActiveUserPreview returns the Preview of the shallowest user-type node
// on the active path. Empty string if no user entry is found.
func firstActiveUserPreview(nodes []sdk.TreeNode) string {
	bestDepth := -1
	best := ""
	for _, n := range nodes {
		if !n.OnActivePath || n.Type != "user" {
			continue
		}
		if bestDepth < 0 || n.Depth < bestDepth {
			bestDepth = n.Depth
			best = n.Preview
		}
	}
	return best
}
