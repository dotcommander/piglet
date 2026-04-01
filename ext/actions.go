package ext

// Action represents an intent requested by an extension command or shortcut.
// Commands enqueue actions; the TUI drains and applies them after the handler returns.
type Action interface {
	isAction()
}

// ActionShowMessage displays a message in the conversation view.
type ActionShowMessage struct{ Text string }

// ActionNotify sends a transient notification.
// Level controls styling: "" or "info" = muted, "warn" = warning, "error" = error.
type ActionNotify struct {
	Message string
	Level   string
}

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

// ActionSetWidget sets or clears persistent multi-line content in a TUI region.
// Placement: "above-input" (between messages and input) or "below-status" (after status bar).
// Empty Content removes the widget. Keyed: last-write-wins per key.
type ActionSetWidget struct {
	Key       string
	Placement string // "above-input" or "below-status"
	Content   string // empty = remove widget
}

// ActionShowOverlay creates or replaces a named overlay.
// Anchor: "center" (default), "right", "left".
// Width: "50%", "80" (chars), "" (auto ~60%).
type ActionShowOverlay struct {
	Key     string
	Title   string
	Content string
	Anchor  string
	Width   string
}

// ActionCloseOverlay removes a named overlay.
type ActionCloseOverlay struct {
	Key string
}

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
func (ActionSetWidget) isAction()       {}
func (ActionShowOverlay) isAction()     {}
func (ActionCloseOverlay) isAction()    {}
