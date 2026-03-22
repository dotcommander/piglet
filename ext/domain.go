package ext

import (
	"fmt"
	"github.com/dotcommander/piglet/core"
)

// ---------------------------------------------------------------------------
// Session domain methods (backed by SessionManager)
// ---------------------------------------------------------------------------

// Sessions returns all sessions, newest first.
func (a *App) Sessions() ([]SessionSummary, error) {
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
	if sm == nil {
		return nil, fmt.Errorf("sessions not configured")
	}
	return sm.List()
}

// LoadSession opens a session by path and enqueues a swap.
func (a *App) LoadSession(path string) error {
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
	if sm == nil {
		return fmt.Errorf("sessions not configured")
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
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
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

// SessionTitle returns the current session's title (empty if not set).
func (a *App) SessionTitle() string {
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
	if sm == nil {
		return ""
	}
	return sm.Title()
}

// SetSessionTitle updates the current session's title.
func (a *App) SetSessionTitle(title string) error {
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
	if sm == nil {
		return fmt.Errorf("no active session")
	}
	return sm.SetTitle(title)
}

// ---------------------------------------------------------------------------
// Model domain methods (backed by ModelManager)
// ---------------------------------------------------------------------------

// AvailableModels returns all registered models.
func (a *App) AvailableModels() []core.Model {
	a.mu.RLock()
	mm := a.models
	a.mu.RUnlock()
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
	a.mu.RLock()
	mm := a.models
	a.mu.RUnlock()
	if mm == nil {
		return 0, fmt.Errorf("model manager not configured")
	}
	return mm.Sync()
}

// ResolveModel returns a model and configured provider for the given model ID
// without switching the main agent. Used by sub-agents to run on different models.
func (a *App) ResolveModel(id string) (core.Model, core.StreamProvider, error) {
	a.mu.RLock()
	mm := a.models
	a.mu.RUnlock()
	if mm == nil {
		return core.Model{}, nil, fmt.Errorf("model manager not configured")
	}
	return mm.Switch(id)
}
