package ext

import (
	"fmt"
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
func (a *App) Steer(content string) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.Steer(&core.UserMessage{Content: content})
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
