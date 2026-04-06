package ext

import (
	"context"
	"fmt"
	"slices"

	"github.com/dotcommander/piglet/core"
)

// ---------------------------------------------------------------------------
// Runtime methods (available after Bind)
// ---------------------------------------------------------------------------

// CWD returns the working directory.
func (a *App) CWD() string { return a.cwd }

// SendMessage enqueues an ActionSendMessage that the TUI will pick up
// and feed into the agent loop as a follow-up user message.
func (a *App) SendMessage(content string) {
	a.enqueue(ActionSendMessage{Content: content})
}

// Steer injects a steering message that interrupts the current turn.
// Returns a SteerDisposition indicating whether the message was delivered,
// queued, or dropped.
func (a *App) Steer(content string) SteerDisposition {
	a.mu.RLock()
	fn := a.steerFn
	agent := a.agent
	a.mu.RUnlock()

	if fn != nil {
		return fn(content)
	}
	if agent != nil {
		agent.Steer(&core.UserMessage{Content: content})
		return SteerDelivered
	}
	return SteerDropped
}

// AbortWithMarker cancels the current agent run and persists an interruption
// marker to the session, so the LLM sees the context on the next run.
// Falls back to plain Steer abort if no marker callback is bound.
func (a *App) AbortWithMarker(reason string) {
	a.mu.RLock()
	fn := a.abortWithMarker
	a.mu.RUnlock()
	if fn != nil {
		fn(reason)
	}
}

// SetModel updates the agent's model.
func (a *App) SetModel(m core.Model) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.SetModel(m)
	}
}

// SetProvider swaps the agent's streaming provider.
func (a *App) SetProvider(p core.StreamProvider) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.SetProvider(p)
	}
}

// Notify sends a notification to the TUI.
func (a *App) Notify(msg string) {
	a.enqueue(ActionNotify{Message: msg})
}

// NotifyWithLevel sends a notification with a severity level.
func (a *App) NotifyWithLevel(msg, level string) {
	a.enqueue(ActionNotify{Message: msg, Level: level})
}

// NotifyWarn sends a warning notification.
func (a *App) NotifyWarn(msg string) {
	a.enqueue(ActionNotify{Message: msg, Level: "warn"})
}

// NotifyError sends an error notification.
func (a *App) NotifyError(msg string) {
	a.enqueue(ActionNotify{Message: msg, Level: "error"})
}

// LastAssistantText returns the text content of the last assistant message,
// or empty string if none found. Walks backward through conversation history.
func (a *App) LastAssistantText() string {
	msgs := a.ConversationMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		am, ok := msgs[i].(*core.AssistantMessage)
		if !ok {
			continue
		}
		for _, c := range am.Content {
			if tc, ok := c.(core.TextContent); ok && tc.Text != "" {
				return tc.Text
			}
		}
	}
	return ""
}

// LLMSnapshot returns a read-only projection of the data that would be sent
// to the LLM in the next API call: system prompt, messages, and tool schemas.
func (a *App) LLMSnapshot() LLMSnapshot {
	a.mu.RLock()
	agent := a.agent
	tools := make([]core.ToolSchema, 0, len(a.tools))
	for _, td := range a.tools {
		tools = append(tools, td.ToolSchema)
	}
	a.mu.RUnlock()

	var snap LLMSnapshot
	snap.Tools = tools
	if agent != nil {
		snap.System = agent.System()
		snap.Messages = agent.Messages()
	}
	return snap
}

// ShowOverlay creates or replaces a named overlay in the TUI.
// Anchor: "center" (default), "right", "left". Width: "50%", "80" (chars), "" (auto).
func (a *App) ShowOverlay(key, title, content, anchor, width string) {
	a.enqueue(ActionShowOverlay{Key: key, Title: title, Content: content, Anchor: anchor, Width: width})
}

// CloseOverlay removes a named overlay.
func (a *App) CloseOverlay(key string) {
	a.enqueue(ActionCloseOverlay{Key: key})
}

// SetWidget sets or clears a persistent multi-line widget in a TUI region.
// Placement: "above-input" or "below-status". Empty content removes the widget.
func (a *App) SetWidget(key, placement, content string) {
	a.enqueue(ActionSetWidget{Key: key, Placement: placement, Content: content})
}

// SetStatus updates a status bar widget.
func (a *App) SetStatus(key, text string) {
	a.enqueue(ActionSetStatus{Key: key, Text: text})
}

// ShowMessage displays a message in the TUI.
func (a *App) ShowMessage(text string) {
	a.enqueue(ActionShowMessage{Text: text})
}

// RequestQuit signals the TUI to quit.
func (a *App) RequestQuit() {
	a.enqueue(ActionQuit{})
}

// ShowPicker shows a picker/modal in the TUI.
func (a *App) ShowPicker(title string, items []PickerItem, onSelect func(PickerItem)) {
	a.enqueue(ActionShowPicker{Title: title, Items: items, OnSelect: onSelect})
}

// Provider returns the current agent's streaming provider.
func (a *App) Provider() core.StreamProvider {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent == nil {
		return nil
	}
	return agent.Provider()
}

// SystemPrompt returns the current agent's system prompt.
func (a *App) SystemPrompt() string {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent == nil {
		return ""
	}
	return agent.System()
}

// RunBackground starts a background agent with the given prompt.
// Returns an error if not bound or if a background agent is already running.
func (a *App) RunBackground(prompt string) error {
	a.mu.RLock()
	fn := a.runBackground
	a.mu.RUnlock()
	if fn == nil {
		return fmt.Errorf("background agent not available")
	}
	return fn(prompt)
}

// CancelBackground stops the running background agent. No-op if not bound or not running.
func (a *App) CancelBackground() {
	a.mu.RLock()
	fn := a.cancelBackground
	a.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// IsBackgroundRunning returns whether a background agent is currently active.
func (a *App) IsBackgroundRunning() bool {
	a.mu.RLock()
	fn := a.isBackgroundRunning
	a.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return false
}

// ConversationMessages returns a snapshot of the conversation history.
func (a *App) ConversationMessages() []core.Message {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		return agent.Messages()
	}
	return nil
}

// SetConversationMessages replaces the conversation history.
func (a *App) SetConversationMessages(msgs []core.Message) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.SetMessages(msgs)
	}
}

// Publish sends data to all subscribers of a topic.
// Callbacks run synchronously — keep them fast or use goroutines in the subscriber.
func (a *App) Publish(topic string, data any) {
	a.mu.RLock()
	subs := make([]eventSub, len(a.eventBus[topic]))
	copy(subs, a.eventBus[topic])
	a.mu.RUnlock()
	for _, sub := range subs {
		sub.fn(data)
	}
}

// ToggleStepMode toggles step mode and returns the new state.
func (a *App) ToggleStepMode() bool {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent == nil {
		return false
	}
	on := !agent.StepMode()
	agent.SetStepMode(on)
	return on
}

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
