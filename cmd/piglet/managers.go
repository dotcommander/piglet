package main

import (
	"context"
	"fmt"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/modelsdev"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
)

// sessionMgr implements ext.SessionManager using the session package.
type sessionMgr struct {
	dir     string
	current **session.Session // pointer to the active session pointer (swappable)
	agent   ext.AgentAPI
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
	if len(parentID) > 8 {
		parentID = parentID[:8]
	}
	return parentID, forked, len(forked.Messages()), nil
}

func (m *sessionMgr) SetTitle(title string) error {
	if *m.current == nil {
		return fmt.Errorf("no active session")
	}
	return (*m.current).SetTitle(title)
}

// modelMgr implements ext.ModelManager using the provider registry.
type modelMgr struct {
	registry *provider.Registry
	auth     *config.Auth
}

func (m *modelMgr) Available() []core.Model {
	return m.registry.Models()
}

func (m *modelMgr) Switch(id string) (core.Model, core.StreamProvider, error) {
	models := m.registry.Models()
	for _, mod := range models {
		if mod.Provider+"/"+mod.ID == id {
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
	return modelsdev.Sync(context.Background(), m.registry, m.auth)
}
