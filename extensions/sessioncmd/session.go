package sessioncmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

func registerSession(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "session",
		Description: "Manage sessions (list, new, tree)",
		Handler: func(ctx context.Context, args string) error {
			args = strings.TrimSpace(args)
			switch {
			case args == "new":
				_, _, err := e.ForkSession(ctx)
				if err != nil {
					e.ShowMessage("Failed to create session: " + err.Error())
					return nil
				}
				if err := e.SetConversationMessages(ctx, nil); err != nil {
					e.ShowMessage("New session created (clear failed: " + err.Error() + ")")
					return nil
				}
				e.ShowMessage("New session created")
				return nil

			case args == "tree":
				return showSessionTree(ctx, e)

			default:
				return openSessionPicker(ctx, e)
			}
		},
	})
}

// openSessionPicker runs the default /session behavior (list + picker + load).
// Shared between the slash command and the ctrl+s shortcut.
func openSessionPicker(ctx context.Context, e *sdk.Extension) error {
	summaries, err := e.Sessions(ctx)
	if err != nil {
		e.ShowMessage(err.Error())
		return nil
	}
	if len(summaries) == 0 {
		e.ShowMessage("No sessions found")
		return nil
	}
	items := sessionPickerItems(summaries)
	selected, err := e.ShowPicker(ctx, "Select Session", items)
	if err != nil || selected == "" {
		// Picker dismissed or timed out; silently return. Do NOT ShowMessage.
		return nil
	}
	if err := e.LoadSession(ctx, selected); err != nil {
		e.ShowMessage("Failed to open session: " + err.Error())
		return nil
	}
	e.ShowMessage("Loaded session: " + selected)
	return nil
}

type sessionTree struct {
	summaries []sdk.SessionInfo
	byID      map[string]int   // session ID → index
	children  map[string][]int // parent ID → child indices
	roots     []int            // indices with no parent (or orphaned parent)
}

func buildSessionTree(summaries []sdk.SessionInfo) sessionTree {
	t := sessionTree{
		summaries: summaries,
		byID:      make(map[string]int, len(summaries)),
		children:  make(map[string][]int),
	}
	for i, s := range summaries {
		t.byID[s.ID] = i
	}
	for i, s := range summaries {
		if s.ParentID != "" {
			if _, ok := t.byID[s.ParentID]; ok {
				t.children[s.ParentID] = append(t.children[s.ParentID], i)
				continue
			}
		}
		t.roots = append(t.roots, i)
	}
	return t
}

// sessionLabel returns the title, or the first 8 chars of the ID if title is empty.
func sessionLabel(title, id string) string {
	if title != "" {
		return title
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func showSessionTree(ctx context.Context, e *sdk.Extension) error {
	summaries, err := e.Sessions(ctx)
	if err != nil {
		e.ShowMessage(err.Error())
		return nil
	}
	if len(summaries) == 0 {
		e.ShowMessage("No sessions found")
		return nil
	}

	tree := buildSessionTree(summaries)

	var b strings.Builder
	b.WriteString("Session tree:\n\n")

	var walk func(idx int, prefix, connector string)
	walk = func(idx int, prefix, connector string) {
		s := tree.summaries[idx]
		label := sessionLabel(s.Title, s.ID)
		// CreatedAt is RFC3339 string; show as-is.
		fmt.Fprintf(&b, "%s%s%s  %s\n", prefix, connector, label, s.CreatedAt)
		kids := tree.children[s.ID]
		for i, childIdx := range kids {
			childPrefix := prefix + "│   "
			childConnector := "├── "
			if i == len(kids)-1 {
				childPrefix = prefix + "    "
				childConnector = "└── "
			}
			walk(childIdx, childPrefix, childConnector)
		}
	}

	for _, rootIdx := range tree.roots {
		walk(rootIdx, "", "")
	}
	e.ShowMessage(b.String())
	return nil
}

func sessionPickerItems(summaries []sdk.SessionInfo) []sdk.PickerItem {
	tree := buildSessionTree(summaries)
	var items []sdk.PickerItem
	var walk func(idx, depth int)
	walk = func(idx, depth int) {
		s := tree.summaries[idx]
		label := sessionLabel(s.Title, s.ID)
		if depth > 0 {
			label = "↳ " + label
		}
		desc := s.CreatedAt
		if s.CWD != "" {
			desc += " — " + s.CWD
		}
		// Note: sdk.SessionInfo has no Messages field; (%d msgs) suffix dropped.
		items = append(items, sdk.PickerItem{ID: s.Path, Label: label, Desc: desc})
		for _, childIdx := range tree.children[s.ID] {
			walk(childIdx, depth+1)
		}
	}
	for _, rootIdx := range tree.roots {
		walk(rootIdx, 0)
	}
	return items
}
