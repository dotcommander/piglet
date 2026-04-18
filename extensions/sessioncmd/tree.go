package sessioncmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

func registerTree(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "tree",
		Description: "Show branching tree of current session",
		Handler: func(ctx context.Context, args string) error {
			nodes, err := e.SessionFullTree(ctx)
			if err != nil {
				e.ShowMessage("Failed to load tree: " + err.Error())
				return nil
			}
			if len(nodes) == 0 {
				e.ShowMessage("No entries in current session")
				return nil
			}
			var b strings.Builder
			b.WriteString("Session tree:\n\n")
			for _, node := range nodes {
				indent := strings.Repeat("  ", node.Depth)
				connector := ""
				if node.Depth > 0 {
					if node.OnActivePath {
						connector = "├─ "
					} else {
						connector = "└─ "
					}
				}
				marker := "  "
				if node.OnActivePath {
					marker = "● "
				}
				label := nodeLabel(node)
				bookmark := ""
				if node.Label != "" {
					bookmark = fmt.Sprintf(" [%s]", node.Label)
				}
				fmt.Fprintf(&b, "%s%s%s%s%s\n", indent, connector, marker, label, bookmark)
			}
			e.ShowMessage(b.String())
			return nil
		},
	})
}

// nodeLabel returns a display label for a tree node.
func nodeLabel(node sdk.TreeNode) string {
	ts := formatShortTimestamp(node.Timestamp)
	switch node.Type {
	case "user":
		preview := node.Preview
		if preview == "" {
			preview = "(empty)"
		}
		return fmt.Sprintf("[user] %s  %s", preview, ts)
	case "assistant":
		preview := node.Preview
		if preview == "" {
			preview = "(response)"
		}
		if len([]rune(preview)) > 40 {
			preview = string([]rune(preview)[:40]) + "..."
		}
		return fmt.Sprintf("[asst] %s  %s", preview, ts)
	case "tool_result":
		return fmt.Sprintf("[tool] %s", ts)
	case "compact":
		return fmt.Sprintf("[compact] %s", ts)
	case "branch_summary":
		return fmt.Sprintf("[branch] %s", ts)
	case "custom_message":
		preview := node.Preview
		if preview == "" {
			preview = "(injected)"
		}
		return fmt.Sprintf("[msg] %s  %s", preview, ts)
	default:
		return fmt.Sprintf("[%s] %s", node.Type, ts)
	}
}
