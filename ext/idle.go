package ext

import (
	"context"
	"slices"
)

// WaitForIdle blocks until SignalIdle is called or the context is cancelled.
// Returns immediately if the agent is already idle (missed-signal safe).
func (a *App) WaitForIdle(ctx context.Context) error {
	a.mu.Lock()
	if a.idle {
		a.mu.Unlock()
		return nil
	}
	ch := make(chan struct{}, 1)
	a.idleWaiters = append(a.idleWaiters, ch)
	a.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		a.mu.Lock()
		a.idleWaiters = slices.DeleteFunc(a.idleWaiters, func(c chan struct{}) bool { return c == ch })
		a.mu.Unlock()
		return ctx.Err()
	}
}

// SignalIdle marks the agent as idle and wakes all pending WaitForIdle callers.
// Called by the shell/TUI when the agent finishes a turn.
func (a *App) SignalIdle() {
	a.mu.Lock()
	a.idle = true
	waiters := a.idleWaiters
	a.idleWaiters = nil
	a.mu.Unlock()
	for _, ch := range waiters {
		close(ch)
	}
}

// ClearIdle marks the agent as no longer idle.
// Must be called when the agent starts a new run, before any WaitForIdle callers register.
func (a *App) ClearIdle() {
	a.mu.Lock()
	a.idle = false
	a.mu.Unlock()
}
