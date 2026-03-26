package ext

import (
	"cmp"
	"context"
	"slices"

	"github.com/dotcommander/piglet/core"
)

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

// RegisterEventHandler adds a handler that reacts to agent lifecycle events.
// Sorted by priority ascending (lower = earlier).
func (a *App) RegisterEventHandler(h EventHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.eventHandlers = append(a.eventHandlers, h)
	slices.SortFunc(a.eventHandlers, func(x, y EventHandler) int { return cmp.Compare(x.Priority, y.Priority) })
}

// DispatchEvent sends an agent event to all registered event handlers.
// Called by the TUI or runPrint as events are drained from the agent channel.
// Handlers run synchronously in priority order. Returned actions are enqueued.
func (a *App) DispatchEvent(ctx context.Context, evt core.Event) {
	a.mu.RLock()
	if len(a.eventHandlers) == 0 {
		a.mu.RUnlock()
		return
	}
	handlers := make([]EventHandler, len(a.eventHandlers))
	copy(handlers, a.eventHandlers)
	a.mu.RUnlock()

	for _, h := range handlers {
		if h.Filter != nil && !h.Filter(evt) {
			continue
		}
		if h.Handle == nil {
			continue
		}
		if action := h.Handle(ctx, evt); action != nil {
			a.enqueue(action)
		}
	}
}
