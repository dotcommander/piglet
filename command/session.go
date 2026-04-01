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

	// Build tree structure
	byID := make(map[string]int, len(summaries))
	children := make(map[string][]int)
	var roots []int

	for i, s := range summaries {
		byID[s.ID] = i
	}
	for i, s := range summaries {
		if s.ParentID != "" {
			if _, ok := byID[s.ParentID]; ok {
				children[s.ParentID] = append(children[s.ParentID], i)
				continue
			}
		}
		roots = append(roots, i)
	}

	var b strings.Builder
	b.WriteString("Session tree:\n\n")

	var walk func(idx int, prefix, connector string)
	walk = func(idx int, prefix, connector string) {
		s := summaries[idx]
		label := s.Title
		if label == "" {
			if len(s.ID) > 8 {
				label = s.ID[:8]
			} else {
				label = s.ID
			}
		}
		ts := s.CreatedAt.Format("01-02 15:04")
		fmt.Fprintf(&b, "%s%s%s  %s  %d msgs\n", prefix, connector, label, ts, s.Messages)

		kids := children[s.ID]
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

	for _, rootIdx := range roots {
		walk(rootIdx, "", "")
	}

	a.ShowMessage(b.String())
	return nil
}

func sessionPickerItems(summaries []ext.SessionSummary) []ext.PickerItem {
	// Build tree: index by ID, group children under parents
	byID := make(map[string]int, len(summaries))
	children := make(map[string][]int) // parentID → child indices
	var roots []int

	for i, s := range summaries {
		byID[s.ID] = i
		if s.ParentID != "" {
			children[s.ParentID] = append(children[s.ParentID], i)
		} else {
			roots = append(roots, i)
		}
	}

	// Orphaned forks (parent deleted) become roots
	for i, s := range summaries {
		if s.ParentID != "" {
			if _, ok := byID[s.ParentID]; !ok {
				roots = append(roots, i)
			}
		}
	}

	// Walk tree depth-first, building picker items with indentation
	var items []ext.PickerItem
	var walk func(idx, depth int)
	walk = func(idx, depth int) {
		s := summaries[idx]
		label := s.Title
		if label == "" {
			if len(s.ID) > 8 {
				label = s.ID[:8]
			} else {
				label = s.ID
			}
		}
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
		for _, childIdx := range children[s.ID] {
			walk(childIdx, depth+1)
		}
	}

	for _, rootIdx := range roots {
		walk(rootIdx, 0)
	}

	return items
}
