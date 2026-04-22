package external

import "encoding/json"

// ---------------------------------------------------------------------------
// Tool execution: host → extension (request/response)
// ---------------------------------------------------------------------------

// ToolExecuteParams is sent when the LLM calls a tool owned by this extension.
type ToolExecuteParams struct {
	CallID string         `json:"callId"`
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
}

// ToolExecuteResult is the extension's response.
type ToolExecuteResult struct {
	Content   []ContentBlock `json:"content"`
	IsError   bool           `json:"isError,omitempty"`
	ErrorCode string         `json:"errorCode,omitempty"` // machine-readable code; empty on success
	ErrorHint string         `json:"errorHint,omitempty"` // actionable hint; empty if not applicable
}

// ContentBlock is a simplified content block for the wire protocol.
type ContentBlock struct {
	Type string `json:"type"` // "text" or "image"
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"`
	Mime string `json:"mime,omitempty"`
}

// ---------------------------------------------------------------------------
// Runtime: extension → host (notifications)
// ---------------------------------------------------------------------------

// NotifyParams sends a notification to the TUI.
type NotifyParams struct {
	Message string `json:"message"`
	Level   string `json:"level,omitempty"` // "", "info" → muted; "warn" → warning; "error" → error
}

// LogParams writes to the host's log.
type LogParams struct {
	Level   string `json:"level"` // "debug", "info", "warn", "error"
	Message string `json:"message"`
}

// ShowMessageParams displays a message in the conversation.
type ShowMessageParams struct {
	Text string `json:"text"`
}

// SendMessageParams is the payload for a sendMessage notification.
type SendMessageParams struct {
	Content string `json:"content"`
}

// SteerParams is the payload for a steer notification.
// Interrupts the current turn and injects a message.
type SteerParams struct {
	Content string `json:"content"`
}

// AbortWithMarkerParams is the payload for aborting with a session marker.
type AbortWithMarkerParams struct {
	Reason string `json:"reason"`
}

// ---------------------------------------------------------------------------
// Command execution: host → extension (request/response)
// ---------------------------------------------------------------------------

// CommandExecuteParams is sent when the user invokes a slash command
// owned by this extension.
type CommandExecuteParams struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

// CommandExecuteResult is the extension's response.
type CommandExecuteResult struct {
	// empty for now; errors use the RPC error field
}

// ---------------------------------------------------------------------------
// Interceptor callbacks: host → extension (request/response)
// ---------------------------------------------------------------------------

// InterceptorBeforeParams is sent when a tool is about to execute.
type InterceptorBeforeParams struct {
	Name     string         `json:"name"` // interceptor discriminator
	ToolName string         `json:"toolName"`
	Args     map[string]any `json:"args"`
}

// InterceptorBeforeResult is the extension's response.
type InterceptorBeforeResult struct {
	Allow   bool           `json:"allow"`
	Args    map[string]any `json:"args,omitempty"`
	Preview string         `json:"preview,omitempty"` // shown to LLM when allow=false
}

// InterceptorAfterParams is sent after tool execution.
type InterceptorAfterParams struct {
	Name     string `json:"name"` // interceptor discriminator
	ToolName string `json:"toolName"`
	Details  any    `json:"details,omitempty"`
}

// InterceptorAfterResult is the extension's response.
type InterceptorAfterResult struct {
	Details any `json:"details,omitempty"`
}

// ---------------------------------------------------------------------------
// Event dispatch: host → extension (request/response)
// ---------------------------------------------------------------------------

// EventDispatchParams is sent when an agent lifecycle event occurs.
type EventDispatchParams struct {
	Type string          `json:"type"` // e.g. "EventAgentEnd", "EventTurnEnd"
	Data json.RawMessage `json:"data,omitempty"`
}

// EventDispatchResult is the extension's response.
type EventDispatchResult struct {
	Action *ActionResult `json:"action,omitempty"`
}

// ---------------------------------------------------------------------------
// Shortcut callback: host → extension (request/response)
// ---------------------------------------------------------------------------

// ShortcutHandleParams is sent when the user presses a registered shortcut.
type ShortcutHandleParams struct {
	Key string `json:"key"` // which shortcut key was pressed
}

// ShortcutHandleResult is the extension's response.
type ShortcutHandleResult struct {
	Action *ActionResult `json:"action,omitempty"`
}

// ---------------------------------------------------------------------------
// Message hook callback: host → extension (request/response)
// ---------------------------------------------------------------------------

// MessageHookParams is sent before a user message reaches the LLM.
type MessageHookParams struct {
	Message string `json:"message"`
}

// MessageHookResult is the extension's response.
type MessageHookResult struct {
	Injection string `json:"injection,omitempty"` // ephemeral context to inject
}

// ---------------------------------------------------------------------------
// Compactor execution: host → extension (request/response)
// ---------------------------------------------------------------------------

// CompactExecuteParams is sent when the token threshold is exceeded.
type CompactExecuteParams struct {
	Messages []CompactMessage `json:"messages"`
}

// CompactMessage wraps a message with a type discriminator for JSON transport.
type CompactMessage struct {
	Type string          `json:"type"` // "user", "assistant", "tool_result"
	Data json.RawMessage `json:"data"`
}

// CompactExecuteResult is the extension's response.
type CompactExecuteResult struct {
	Messages []CompactMessage `json:"messages"`
}

// ---------------------------------------------------------------------------
// Serializable action (replaces Go Action interface over the wire)
// ---------------------------------------------------------------------------

// ActionResult is a JSON-serializable action returned by extensions.
// The host converts it to the appropriate ext.Action type.
type ActionResult struct {
	Type    string          `json:"type"`              // "notify", "showMessage", "setSessionTitle", "quit"
	Payload json.RawMessage `json:"payload,omitempty"` // type-specific data
}

// SetWidgetParams sets or clears a persistent widget in the TUI.
type SetWidgetParams struct {
	Key       string `json:"key"`
	Placement string `json:"placement"` // "above-input" or "below-status"
	Content   string `json:"content"`   // empty = remove
}

// ShowOverlayParams creates or replaces a named overlay.
type ShowOverlayParams struct {
	Key     string `json:"key"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
	Anchor  string `json:"anchor,omitempty"` // "center" (default), "right", "left"
	Width   string `json:"width,omitempty"`  // "50%", "80" (chars), "" (auto)
}

// CloseOverlayParams removes a named overlay.
type CloseOverlayParams struct {
	Key string `json:"key"`
}
