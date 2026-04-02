package main

import (
	"fmt"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
)

// sessionMgr implements ext.SessionManager using the session package.
type sessionMgr struct {
	dir     string
	current **session.Session // pointer to the active session pointer (swappable)
}

func (m *sessionMgr) List() ([]ext.SessionSummary, error) {
	if m.dir == "" {
		return nil, fmt.Errorf("sessions not configured")
	}
	summaries, err := session.List(m.dir)
	if err != nil {
		return nil, err
	}
	out := make([]ext.SessionSummary, len(summaries))
	for i, s := range summaries {
		out[i] = ext.SessionSummary{
			ID:        s.ID,
			Path:      s.Path,
			Title:     s.Title,
			Model:     s.Model,
			CWD:       s.CWD,
			CreatedAt: s.CreatedAt,
			Messages:  s.Messages,
			ParentID:  s.ParentID,
		}
	}
	return out, nil
}

func (m *sessionMgr) Load(path string) (any, error) {
	sess, err := session.Open(path)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (m *sessionMgr) Fork() (string, any, int, error) {
	if *m.current == nil {
		return "", nil, 0, fmt.Errorf("no active session")
	}
	forked, err := (*m.current).Fork(0)
	if err != nil {
		return "", nil, 0, err
	}
	parentID := (*m.current).ID()
	return parentID, forked, len(forked.Messages()), nil
}

func (m *sessionMgr) Branch(entryID string) (any, error) {
	if *m.current == nil {
		return nil, fmt.Errorf("no active session")
	}
	if err := (*m.current).Branch(entryID); err != nil {
		return nil, err
	}
	return *m.current, nil
}

func (m *sessionMgr) BranchWithSummary(entryID, summary string) (any, error) {
	if *m.current == nil {
		return nil, fmt.Errorf("no active session")
	}
	if err := (*m.current).BranchWithSummary(entryID, summary); err != nil {
		return nil, err
	}
	return *m.current, nil
}

func (m *sessionMgr) EntryInfos() []ext.EntryInfo {
	if *m.current == nil {
		return nil
	}
	raw := (*m.current).EntryInfos()
	out := make([]ext.EntryInfo, len(raw))
	for i, e := range raw {
		out[i] = ext.EntryInfo{
			ID:        e.ID,
			ParentID:  e.ParentID,
			Type:      e.Type,
			Timestamp: e.Timestamp,
			Children:  e.Children,
		}
	}
	return out
}

func (m *sessionMgr) SetTitle(title string) error {
	if *m.current == nil {
		return fmt.Errorf("no active session")
	}
	return (*m.current).SetTitle(title)
}

func (m *sessionMgr) Title() string {
	if *m.current == nil {
		return ""
	}
	return (*m.current).Meta().Title
}

func (m *sessionMgr) AppendEntry(kind string, data any) error {
	if *m.current == nil {
		return fmt.Errorf("no active session")
	}
	return (*m.current).AppendCustom(kind, data)
}

func (m *sessionMgr) AppendCustomMessage(role, content string) error {
	if *m.current == nil {
		return fmt.Errorf("no active session")
	}
	return (*m.current).AppendCustomMessage(role, content)
}

func (m *sessionMgr) AppendLabel(targetID, label string) error {
	if *m.current == nil {
		return fmt.Errorf("no active session")
	}
	return (*m.current).AppendLabel(targetID, label)
}

func (m *sessionMgr) FullTree() []ext.TreeNode {
	if *m.current == nil {
		return nil
	}
	raw := (*m.current).FullTree()
	out := make([]ext.TreeNode, len(raw))
	for i, n := range raw {
		out[i] = ext.TreeNode{
			ID:           n.ID,
			ParentID:     n.ParentID,
			Type:         n.Type,
			Timestamp:    n.Timestamp,
			Children:     n.Children,
			OnActivePath: n.OnActivePath,
			Depth:        n.Depth,
			Preview:      n.Preview,
			Label:        n.Label,
		}
	}
	return out
}

// modelMgr implements ext.ModelManager using the provider registry.
type modelMgr struct {
	registry     *provider.Registry
	auth         *config.Auth
	app          *ext.App
	localServers []string
	localCtxWin  int
	localMaxTok  int
}

func newModelMgr(rt *runtime, app *ext.App) *modelMgr {
	return &modelMgr{
		registry:     rt.registry,
		auth:         rt.auth,
		app:          app,
		localServers: rt.settings.LocalServers,
		localCtxWin:  config.IntOr(rt.settings.LocalDefaults.ContextWindow, provider.LocalDefaultContextWindow()),
		localMaxTok:  config.IntOr(rt.settings.LocalDefaults.MaxTokens, provider.LocalDefaultMaxTokens()),
	}
}

func (m *modelMgr) Available() []core.Model {
	return m.registry.Models()
}

func (m *modelMgr) Switch(id string) (core.Model, core.StreamProvider, error) {
	models := m.registry.Models()
	for _, mod := range models {
		if mod.Provider+"/"+mod.ID == id {
			// Check for external provider first
			if m.app != nil {
				if p, ok := m.app.StreamProvider(string(mod.API), mod); ok {
					return mod, p, nil
				}
			}
			apiKeyFn := func() string {
				return m.auth.GetAPIKey(mod.Provider)
			}
			prov, err := m.registry.Create(mod, apiKeyFn)
			if err != nil {
				return core.Model{}, nil, fmt.Errorf("create provider: %w", err)
			}
			return mod, prov, nil
		}
	}
	return core.Model{}, nil, fmt.Errorf("unknown model: %s", id)
}

func (m *modelMgr) Sync() (int, error) {
	n, err := m.registry.ReloadModels()
	if len(m.localServers) > 0 {
		n += m.registry.RegisterLocalServers(m.localServers, m.localCtxWin, m.localMaxTok)
	}
	return n, err
}
