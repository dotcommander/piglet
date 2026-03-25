package ext

// Action represents an intent requested by an extension command or shortcut.
// Commands enqueue actions; the TUI drains and applies them after the handler returns.
type Action interface {
	isAction()
}

// ActionShowMessage displays a message in the conversation view.
type ActionShowMessage struct{ Text string }

// ActionNotify sends a transient notification.
type ActionNotify struct{ Message string }

// ActionQuit signals the TUI to exit.
type ActionQuit struct{}

// ActionSetStatus updates a status bar widget (e.g. "model").
type ActionSetStatus struct{ Key, Text string }

// ActionShowPicker opens a modal picker.
type ActionShowPicker struct {
	Title    string
	Items    []PickerItem
	OnSelect func(PickerItem)
}

// ActionSwapSession replaces the active session.
// Session is typed as any to avoid importing session/ from ext/.
type ActionSwapSession struct{ Session any }

// ActionSetSessionTitle sets the current session's title.
type ActionSetSessionTitle struct{ Title string }

// ActionRunAsync runs Fn in a goroutine. The returned Action (if non-nil)
// is enqueued when Fn completes. Use for expensive event handler work.
type ActionRunAsync struct{ Fn func() Action }

// ActionAttachImage attaches an image to the next message.
// Image is typed as any to avoid importing core.ImageContent.
type ActionAttachImage struct{ Image any }

// ActionDetachImage removes a pending image attachment.
type ActionDetachImage struct{}

// ActionSendMessage injects a user message into the agent loop.
type ActionSendMessage struct{ Content string }

// ActionExec hands the terminal to an external process (e.g., $EDITOR).
// Cmd is typed as any to avoid importing os/exec from ext/.
// The TUI asserts *exec.Cmd and uses tea.ExecProcess.
type ActionExec struct{ Cmd any }

func (ActionShowMessage) isAction()     {}
func (ActionNotify) isAction()          {}
func (ActionQuit) isAction()            {}
func (ActionSetStatus) isAction()       {}
func (ActionShowPicker) isAction()      {}
func (ActionSwapSession) isAction()     {}
func (ActionSetSessionTitle) isAction() {}
func (ActionRunAsync) isAction()        {}
func (ActionAttachImage) isAction()     {}
func (ActionDetachImage) isAction()     {}
func (ActionSendMessage) isAction()     {}
func (ActionExec) isAction()            {}
