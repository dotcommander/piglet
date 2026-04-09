package ext

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
