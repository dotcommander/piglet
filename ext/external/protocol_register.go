package external

import "encoding/json"

// ---------------------------------------------------------------------------
// Registration: extension → host (notifications, no response expected)
// ---------------------------------------------------------------------------

// RegisterToolParams registers a tool the LLM can call.
type RegisterToolParams struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	Parameters        any    `json:"parameters"` // JSON Schema
	PromptHint        string `json:"promptHint,omitempty"`
	Deferred          bool   `json:"deferred,omitempty"`
	InterruptBehavior string `json:"interruptBehavior,omitempty"` // "cancel" (default) or "block"
}

// RegisterCommandParams registers a slash command.
type RegisterCommandParams struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Immediate   bool   `json:"immediate,omitempty"`
}

// RegisterPromptSectionParams adds a section to the system prompt.
type RegisterPromptSectionParams struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Order     int    `json:"order,omitempty"`
	TokenHint int    `json:"tokenHint,omitzero"`
}

// RegisterInterceptorParams registers a before/after tool interceptor.
type RegisterInterceptorParams struct {
	Name     string `json:"name"`
	Priority int    `json:"priority,omitempty"`
}

// RegisterEventHandlerParams registers an agent lifecycle event handler.
type RegisterEventHandlerParams struct {
	Name     string   `json:"name"`
	Priority int      `json:"priority,omitempty"`
	Events   []string `json:"events,omitempty"` // nil = all events
}

// RegisterShortcutParams registers a keyboard shortcut.
type RegisterShortcutParams struct {
	Key         string `json:"key"` // e.g. "ctrl+g"
	Description string `json:"description"`
}

// RegisterMessageHookParams registers a pre-message hook.
type RegisterMessageHookParams struct {
	Name     string `json:"name"`
	Priority int    `json:"priority,omitempty"`
}

// RegisterCompactorParams registers a conversation compactor.
type RegisterCompactorParams struct {
	Name      string `json:"name"`
	Threshold int    `json:"threshold,omitempty"` // 0 = use config default
}

// ---------------------------------------------------------------------------
// Input transformer registration + callback
// ---------------------------------------------------------------------------

// RegisterInputTransformerParams registers an input transformer.
type RegisterInputTransformerParams struct {
	Name     string `json:"name"`
	Priority int    `json:"priority,omitempty"`
}

// InputTransformParams is the host → extension callback when user input arrives.
type InputTransformParams struct {
	Input string `json:"input"`
}

// InputTransformResult is the extension's response.
type InputTransformResult struct {
	Output  string `json:"output"`
	Handled bool   `json:"handled"` // true = input was consumed, stop processing
}

// ---------------------------------------------------------------------------
// Provider streaming
// ---------------------------------------------------------------------------

// RegisterProviderParams declares the extension can handle a provider API type.
type RegisterProviderParams struct {
	API string `json:"api"` // "openai", "anthropic", "google"
}

// ProviderStreamParams is the request to stream an LLM call.
type ProviderStreamParams struct {
	RequestID int             `json:"requestId"`
	Model     json.RawMessage `json:"model"`
	System    string          `json:"system"`
	Messages  json.RawMessage `json:"messages"`
	Tools     json.RawMessage `json:"tools,omitempty"`
	Options   json.RawMessage `json:"options,omitempty"`
}

// ProviderStreamResult is the final response after streaming completes.
type ProviderStreamResult struct {
	Message json.RawMessage `json:"message"`
	Error   string          `json:"error,omitempty"`
}

// ProviderDeltaParams is a streaming notification correlated by requestId.
type ProviderDeltaParams struct {
	RequestID int               `json:"requestId"`
	Type      string            `json:"type"`
	Index     int               `json:"index,omitempty"`
	Delta     string            `json:"delta,omitempty"`
	Tool      *ProviderToolCall `json:"tool,omitempty"`
}

// ProviderToolCall is a tool call in a provider delta (toolcall_end event).
type ProviderToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
