package command

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

func registerModel(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "model",
		Description: "Switch model",
		Immediate:   true,
		Handler: func(args string, a *ext.App) error {
			models := a.AvailableModels()
			if len(models) == 0 {
				a.ShowMessage("No models available")
				return nil
			}
			items := make([]ext.PickerItem, len(models))
			for i, mod := range models {
				items[i] = ext.PickerItem{
					ID:    mod.Provider + "/" + mod.ID,
					Label: mod.Name,
					Desc:  mod.Provider,
				}
			}
			a.ShowPicker("Select Model", items, func(selected ext.PickerItem) {
				if err := a.SwitchModel(selected.ID); err != nil {
					a.ShowMessage("Failed to switch model: " + err.Error())
					return
				}
				if cfg, err := config.Load(); err == nil {
					cfg.DefaultModel = selected.ID
					if err := config.Save(cfg); err != nil {
						a.ShowMessage("Switched to " + selected.Label + " (failed to save: " + err.Error() + ")")
						return
					}
				}
				a.ShowMessage("Switched to " + selected.Label)
			})
			return nil
		},
	})
}

func registerSession(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "session",
		Description: "Manage sessions (list, new, tree)",
		Handler: func(args string, a *ext.App) error {
			args = strings.TrimSpace(args)

			switch {
			case args == "new":
				// Fork with 0 messages = fresh session
				_, _, err := a.ForkSession()
				if err != nil {
					a.ShowMessage("Failed to create session: " + err.Error())
					return nil
				}
				a.SetConversationMessages(nil)
				a.ShowMessage("New session created")
				return nil

			case args == "tree":
				return showSessionTree(a)

			default:
				// Picker (existing behavior)
				summaries, err := a.Sessions()
				if err != nil {
					a.ShowMessage(err.Error())
					return nil
				}
				if len(summaries) == 0 {
					a.ShowMessage("No sessions found")
					return nil
				}
				items := sessionPickerItems(summaries)
				a.ShowPicker("Select Session", items, func(selected ext.PickerItem) {
					if err := a.LoadSession(selected.ID); err != nil {
						a.ShowMessage("Failed to open session: " + err.Error())
						return
					}
					a.ShowMessage("Loaded session: " + selected.Label)
				})
				return nil
			}
		},
	})
}

type sessionTree struct {
	summaries []ext.SessionSummary
	byID      map[string]int   // session ID → index
	children  map[string][]int // parent ID → child indices
	roots     []int            // indices with no parent (or orphaned parent)
}

func buildSessionTree(summaries []ext.SessionSummary) sessionTree {
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

func showSessionTree(a *ext.App) error {
	summaries, err := a.Sessions()
	if err != nil {
		a.ShowMessage(err.Error())
		return nil
	}
	if len(summaries) == 0 {
		a.ShowMessage("No sessions found")
		return nil
	}

	tree := buildSessionTree(summaries)

	var b strings.Builder
	b.WriteString("Session tree:\n\n")

	var walk func(idx int, prefix, connector string)
	walk = func(idx int, prefix, connector string) {
		s := tree.summaries[idx]
		label := sessionLabel(s.Title, s.ID)
		ts := s.CreatedAt.Format("01-02 15:04")
		fmt.Fprintf(&b, "%s%s%s  %s  %d msgs\n", prefix, connector, label, ts, s.Messages)

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

	a.ShowMessage(b.String())
	return nil
}

func registerBranch(app *ext.App) {
	app.RegisterCommand(&ext.Command{
		Name:        "branch",
		Description: "Branch to an earlier point in this session",
		Handler: func(args string, a *ext.App) error {
			infos := a.SessionEntryInfos()
			if len(infos) == 0 {
				a.ShowMessage("No entries in current session")
				return nil
			}

			items := make([]ext.PickerItem, len(infos))
			for i, info := range infos {
				label := fmt.Sprintf("[%s] %s", info.Type, info.ID[:min(8, len(info.ID))])
				desc := info.Timestamp.Format("15:04:05")
				if info.Children > 1 {
					desc += fmt.Sprintf(" (%d branches)", info.Children)
				}
				items[i] = ext.PickerItem{
					ID:    info.ID,
					Label: label,
					Desc:  desc,
				}
			}

			a.ShowPicker("Branch to entry", items, func(selected ext.PickerItem) {
				if err := a.BranchSession(selected.ID); err != nil {
					a.ShowMessage("Branch failed: " + err.Error())
					return
				}
				a.ShowMessage("Branched to " + selected.ID)
			})
			return nil
		},
	})
}

func sessionPickerItems(summaries []ext.SessionSummary) []ext.PickerItem {
	tree := buildSessionTree(summaries)

	// Walk tree depth-first, building picker items with indentation
	var items []ext.PickerItem
	var walk func(idx, depth int)
	walk = func(idx, depth int) {
		s := tree.summaries[idx]
		label := sessionLabel(s.Title, s.ID)
		if depth > 0 {
			label = "↳ " + label
		}
		desc := s.CreatedAt.Format("2006-01-02 15:04")
		if s.CWD != "" {
			desc += " — " + s.CWD
		}
		if s.Messages > 0 {
			desc += fmt.Sprintf(" (%d msgs)", s.Messages)
		}
		items = append(items, ext.PickerItem{
			ID:    s.Path,
			Label: label,
			Desc:  desc,
		})
		for _, childIdx := range tree.children[s.ID] {
			walk(childIdx, depth+1)
		}
	}

	for _, rootIdx := range tree.roots {
		walk(rootIdx, 0)
	}

	return items
}
