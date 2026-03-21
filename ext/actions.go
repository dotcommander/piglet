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

func (ActionShowMessage) isAction() {}
func (ActionNotify) isAction()      {}
func (ActionQuit) isAction()        {}
func (ActionSetStatus) isAction()   {}
func (ActionShowPicker) isAction()  {}
func (ActionSwapSession) isAction() {}
