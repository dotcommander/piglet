package shell

import "github.com/dotcommander/piglet/ext"

// NotificationKind discriminates notification types.
type NotificationKind int

const (
	NotifyMessage      NotificationKind = iota // text message to display
	NotifyWarn                                 // warning-level message
	NotifyError                                // error-level message
	NotifyStatus                               // status bar update (key + text)
	NotifySessionSwap                          // session was swapped
	NotifyQuit                                 // frontend should exit
	NotifyPicker                               // modal picker request
	NotifyImage                                // image attach/detach
	NotifyWidget                               // widget set/clear
	NotifyOverlay                              // overlay show/close
	NotifyExec                                 // hand terminal to external process
	NotifySendMessage                          // content should be submitted as user input
	NotifySessionTitle                         // session title changed
	NotifyClearDisplay                         // frontend should clear conversation display
	NotifyQueuedSubmit                         // queued user message was submitted to agent
)

// Notification is a frontend-relevant side-effect from Shell processing.
// Simple frontends switch on Kind and use Text/Key.
// Rich frontends (TUI) type-assert Action for full details.
type Notification struct {
	Kind   NotificationKind
	Text   string     // for Message, Warn, Error, SessionTitle
	Key    string     // for Status, Widget, Overlay
	Action ext.Action // raw action for rich frontends (may be nil)
}
