package ext

import (
	"fmt"
	"github.com/dotcommander/piglet/core"
)

// sessionMgr returns the session manager under RLock, or nil.
func (a *App) sessionMgr() SessionManager {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessions
}

// modelMgr returns the model manager under RLock, or nil.
func (a *App) modelMgr() ModelManager {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.models
}

// ---------------------------------------------------------------------------
// Session domain methods (backed by SessionManager)
// ---------------------------------------------------------------------------

// Sessions returns all sessions, newest first.
func (a *App) Sessions() ([]SessionSummary, error) {
	sm := a.sessionMgr()
	if sm == nil {
		return nil, fmt.Errorf("no active session")
	}
	return sm.List()
}

// LoadSession opens a session by path and enqueues a swap.
func (a *App) LoadSession(path string) error {
	sm := a.sessionMgr()
	if sm == nil {
		return fmt.Errorf("no active session")
	}
	sess, err := sm.Load(path)
	if err != nil {
		return err
	}
	a.enqueue(ActionSwapSession{Session: sess})
	return nil
}

// ForkSession forks the current session into a new branch.
func (a *App) ForkSession() (string, int, error) {
	sm := a.sessionMgr()
	if sm == nil {
		return "", 0, fmt.Errorf("no active session")
	}
	parentID, forked, count, err := sm.Fork()
	if err != nil {
		return "", 0, err
	}
	a.enqueue(ActionSwapSession{Session: forked})
	return parentID, count, nil
}

// BranchSession moves the current session's leaf to an earlier entry (in-place).
// The agent's message history and TUI are refreshed via ActionSwapSession.
func (a *App) BranchSession(entryID string) error {
	return a.BranchSessionWithSummary(entryID, "")
}

// BranchSessionWithSummary moves the leaf and writes a branch_summary entry.
func (a *App) BranchSessionWithSummary(entryID, summary string) error {
	sm := a.sessionMgr()
	if sm == nil {
		return fmt.Errorf("no active session")
	}
	sess, err := sm.BranchWithSummary(entryID, summary)
	if err != nil {
		return err
	}
	a.enqueue(ActionSwapSession{Session: sess})
	return nil
}

// SessionEntryInfos returns entry info for the current branch (for display/picker).
func (a *App) SessionEntryInfos() []EntryInfo {
	sm := a.sessionMgr()
	if sm == nil {
		return nil
	}
	return sm.EntryInfos()
}

// SessionTitle returns the current session's title (empty if not set).
func (a *App) SessionTitle() string {
	sm := a.sessionMgr()
	if sm == nil {
		return ""
	}
	return sm.Title()
}

// SetSessionTitle updates the current session's title.
func (a *App) SetSessionTitle(title string) error {
	sm := a.sessionMgr()
	if sm == nil {
		return fmt.Errorf("no active session")
	}
	return sm.SetTitle(title)
}

// AppendSessionEntry writes a custom extension entry to the current session.
// The kind should be namespaced (e.g., "ext:memory:facts").
func (a *App) AppendSessionEntry(kind string, data any) error {
	sm := a.sessionMgr()
	if sm == nil {
		return fmt.Errorf("no active session")
	}
	return sm.AppendEntry(kind, data)
}

// SetSessionLabel sets or clears a bookmark label on a session entry.
func (a *App) SetSessionLabel(targetID, label string) error {
	sm := a.sessionMgr()
	if sm == nil {
		return fmt.Errorf("no active session")
	}
	return sm.AppendLabel(targetID, label)
}

// SessionFullTree returns the full DAG of the current session for tree rendering.
func (a *App) SessionFullTree() []TreeNode {
	sm := a.sessionMgr()
	if sm == nil {
		return nil
	}
	return sm.FullTree()
}

// AppendCustomMessage writes a message that persists and appears in Messages() on reload.
// Role must be "user" or "assistant".
func (a *App) AppendCustomMessage(role, content string) error {
	sm := a.sessionMgr()
	if sm == nil {
		return fmt.Errorf("no active session")
	}
	return sm.AppendCustomMessage(role, content)
}

// ---------------------------------------------------------------------------
// Model domain methods (backed by ModelManager)
// ---------------------------------------------------------------------------

// AvailableModels returns all registered models.
func (a *App) AvailableModels() []core.Model {
	mm := a.modelMgr()
	if mm == nil {
		return nil
	}
	return mm.Available()
}

// SwitchModel activates a model by its "provider/id" key.
// Updates the agent's model and provider, and enqueues a status update.
func (a *App) SwitchModel(id string) error {
	a.mu.RLock()
	mm := a.models
	agent := a.agent
	a.mu.RUnlock()
	if mm == nil {
		return fmt.Errorf("model manager not configured")
	}
	mod, prov, err := mm.Switch(id)
	if err != nil {
		return err
	}
	if agent != nil {
		agent.SetModel(mod)
		agent.SetProvider(prov)
	}
	a.enqueue(ActionSetStatus{Key: "model", Text: mod.DisplayName()})
	return nil
}

// SyncModels updates the model catalog from an external source.
func (a *App) SyncModels() (int, error) {
	mm := a.modelMgr()
	if mm == nil {
		return 0, fmt.Errorf("model manager not configured")
	}
	return mm.Sync()
}

// WriteModelsWithOverrides regenerates models.yaml from the embedded curated
// list with API-sourced overrides, writes to disk, and reloads the registry.
func (a *App) WriteModelsWithOverrides(overrides map[string]ModelOverride) (int, error) {
	mm := a.modelMgr()
	if mm == nil {
		return 0, fmt.Errorf("model manager not configured")
	}
	return mm.WriteWithOverrides(overrides)
}

// ResolveModel returns a model and configured provider for the given model ID
// without switching the main agent. Used by sub-agents to run on different models.
func (a *App) ResolveModel(id string) (core.Model, core.StreamProvider, error) {
	mm := a.modelMgr()
	if mm == nil {
		return core.Model{}, nil, fmt.Errorf("model manager not configured")
	}
	return mm.Switch(id)
}
