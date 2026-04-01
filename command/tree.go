package command

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/ext"
)

func registerTree(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "tree",
		Description: "Show branching tree of current session",
		Handler: func(args string, a *ext.App) error {
			nodes := a.SessionFullTree()
			if len(nodes) == 0 {
				a.ShowMessage("No entries in current session")
				return nil
			}

			var b strings.Builder
			b.WriteString("Session tree:\n\n")

			for _, node := range nodes {
				indent := strings.Repeat("  ", node.Depth)

				// Connector
				connector := ""
				if node.Depth > 0 {
					if node.OnActivePath {
						connector = "├─ "
					} else {
						connector = "└─ "
					}
				}

				// Active marker
				marker := "  "
				if node.OnActivePath {
					marker = "● "
				}

				// Label
				label := nodeLabel(node)

				// Bookmark
				bookmark := ""
				if node.Label != "" {
					bookmark = fmt.Sprintf(" [%s]", node.Label)
				}

				fmt.Fprintf(&b, "%s%s%s%s%s\n", indent, connector, marker, label, bookmark)
			}

			a.ShowMessage(b.String())
			return nil
		},
	})
}

// nodeLabel returns a display label for a tree node.
func nodeLabel(node ext.TreeNode) string {
	ts := node.Timestamp.Format("15:04:05")

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
		// Extension entries (e.g., "ext:memory:facts")
		return fmt.Sprintf("[%s] %s", node.Type, ts)
	}
}
