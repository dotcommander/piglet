package sdk

// ---------------------------------------------------------------------------
// Notification API (extension → host TUI)
// ---------------------------------------------------------------------------

// Notify sends a notification to the host TUI.
func (e *Extension) Notify(msg string) {
	e.sendNotification("notify", map[string]string{"message": msg})
}

// ShowOverlay creates or replaces a named overlay in the TUI.
// Anchor: "center" (default), "right", "left". Width: "50%", "80" (chars), "" (auto).
func (e *Extension) ShowOverlay(key, title, content, anchor, width string) {
	e.sendNotification("showOverlay", map[string]string{
		"key":     key,
		"title":   title,
		"content": content,
		"anchor":  anchor,
		"width":   width,
	})
}

// CloseOverlay removes a named overlay by key.
func (e *Extension) CloseOverlay(key string) {
	e.sendNotification("closeOverlay", map[string]string{"key": key})
}

// SetWidget sets or clears a persistent multi-line widget in the TUI.
// Placement: "above-input" or "below-status". Empty content removes the widget.
func (e *Extension) SetWidget(key, placement, content string) {
	e.sendNotification("setWidget", map[string]string{
		"key":       key,
		"placement": placement,
		"content":   content,
	})
}

// NotifyWarn sends a warning-level notification.
func (e *Extension) NotifyWarn(msg string) {
	e.sendNotification("notify", map[string]string{"message": msg, "level": "warn"})
}

// NotifyError sends an error-level notification.
func (e *Extension) NotifyError(msg string) {
	e.sendNotification("notify", map[string]string{"message": msg, "level": "error"})
}

// ShowMessage displays a message in the conversation.
func (e *Extension) ShowMessage(text string) {
	e.sendNotification("showMessage", map[string]string{"text": text})
}

// SendMessage injects a user message into the agent loop.
// The message is queued and delivered after the current turn completes.
func (e *Extension) SendMessage(content string) {
	e.sendNotification("sendMessage", map[string]string{"content": content})
}

// Steer interrupts the current turn and injects a message.
// Remaining tool calls are cancelled and this message is processed next.
func (e *Extension) Steer(content string) {
	e.sendNotification("steer", map[string]string{"content": content})
}

// AbortWithMarker cancels the current agent run and persists an interruption
// marker to the session, so the LLM sees the context on the next run.
func (e *Extension) AbortWithMarker(reason string) {
	e.sendNotification("abortWithMarker", map[string]string{"reason": reason})
}

// Log sends a log message to the host.
func (e *Extension) Log(level, msg string) {
	e.sendNotification("log", map[string]string{"level": level, "message": msg})
}
