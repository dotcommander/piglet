package sdk

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// ToolDef defines a tool the LLM can call.
type ToolDef struct {
	Name              string
	Description       string
	Parameters        any // JSON Schema
	PromptHint        string
	Deferred          bool
	InterruptBehavior string // "cancel" (default) or "block"
	Execute           func(ctx context.Context, args map[string]any) (*ToolResult, error)
}

// ToolResult is the return value from a tool execution.
type ToolResult struct {
	Content []ContentBlock
	IsError bool
}

// ContentBlock is a single content element in a tool result.
type ContentBlock struct {
	Type string // "text" or "image"
	Text string
	Data string // base64 image data
	Mime string // MIME type for images
}

// TextResult is a convenience constructor for a text-only tool result.
func TextResult(text string) *ToolResult {
	return &ToolResult{Content: []ContentBlock{{Type: "text", Text: text}}}
}

// ErrorResult is a convenience constructor for an error tool result.
func ErrorResult(text string) *ToolResult {
	return &ToolResult{Content: []ContentBlock{{Type: "text", Text: text}}, IsError: true}
}

// CommandDef defines a slash command.
type CommandDef struct {
	Name        string
	Description string
	Immediate   bool
	Handler     func(ctx context.Context, args string) error
}

// PromptSectionDef defines a system prompt section.
type PromptSectionDef struct {
	Title     string
	Content   string
	Order     int
	TokenHint int // estimated token count; 0 = unknown
}

// InterceptorDef defines a before/after tool interceptor.
type InterceptorDef struct {
	Name     string
	Priority int
	Before   func(ctx context.Context, toolName string, args map[string]any) (allow bool, modified map[string]any, err error)
	After    func(ctx context.Context, toolName string, details any) (any, error)
	Preview  func(ctx context.Context, toolName string, args map[string]any) string // optional: called when Before blocks
}

// EventHandlerDef defines an agent lifecycle event handler.
type EventHandlerDef struct {
	Name     string
	Priority int
	Events   []string // nil = all events
	Handle   func(ctx context.Context, eventType string, data json.RawMessage) *Action
}

// ShortcutDef defines a keyboard shortcut handler.
type ShortcutDef struct {
	Key         string // e.g. "ctrl+g"
	Description string
	Handler     func(ctx context.Context) (*Action, error)
}

// MessageHookDef defines a pre-message hook.
type MessageHookDef struct {
	Name      string
	Priority  int
	OnMessage func(ctx context.Context, msg string) (string, error)
}

// InputTransformerDef defines an input transformer that intercepts user input.
type InputTransformerDef struct {
	Name      string
	Priority  int
	Transform func(ctx context.Context, input string) (output string, handled bool, err error)
}

// CompactorDef defines a conversation compactor.
type CompactorDef struct {
	Name      string
	Threshold int // 0 = use config default
	Compact   func(ctx context.Context, messages json.RawMessage) (json.RawMessage, error)
}

// Action represents a result action to return to the host.
type Action struct {
	Type    string
	Payload any
}

// Action constructors
func ActionNotify(msg string) *Action {
	return &Action{Type: "notify", Payload: map[string]string{"message": msg}}
}

func ActionShowMessage(text string) *Action {
	return &Action{Type: "showMessage", Payload: map[string]string{"text": text}}
}

func ActionSetSessionTitle(title string) *Action {
	return &Action{Type: "setSessionTitle", Payload: map[string]string{"title": title}}
}

func ActionSetStatus(key, text string) *Action {
	return &Action{Type: "setStatus", Payload: map[string]string{"key": key, "text": text}}
}

func ActionAttachImage(data, mimeType string) *Action {
	return &Action{Type: "attachImage", Payload: map[string]string{"data": data, "mimeType": mimeType}}
}

func ActionSendMessage(content string) *Action {
	return &Action{Type: "sendMessage", Payload: map[string]string{"content": content}}
}

// ---------------------------------------------------------------------------
// Host service types (extension → host requests)
// ---------------------------------------------------------------------------

// ChatMessage is a single message in a chat request.
type ChatMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// ChatRequest asks the host to make an LLM call.
type ChatRequest struct {
	System    string        `json:"system,omitempty"`
	Messages  []ChatMessage `json:"messages"`
	Model     string        `json:"model,omitempty"`     // "small", "default", or explicit model ID
	MaxTokens int           `json:"maxTokens,omitempty"` // 0 = provider default
}

// ChatResponse is the host's response to a chat request.
type ChatResponse struct {
	Text  string     `json:"text"`
	Usage TokenUsage `json:"usage"`
}

// TokenUsage reports token consumption.
type TokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

// AgentRequest asks the host to run a full agent loop.
type AgentRequest struct {
	System   string `json:"system,omitempty"`
	Task     string `json:"task"`
	Tools    string `json:"tools,omitempty"`    // "background_safe" (default) or "all"
	Model    string `json:"model,omitempty"`    // "small", "default", or explicit model ID
	MaxTurns int    `json:"maxTurns,omitempty"` // 0 = use config default
}

// AgentResponse is the host's response to an agent run.
type AgentResponse struct {
	Text  string     `json:"text"`
	Turns int        `json:"turns"`
	Usage TokenUsage `json:"usage"`
}

// HostToolInfo describes a host-registered tool.
type HostToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
	Deferred    bool   `json:"deferred,omitempty"`
}

// HostTool is a thin tool wrapper that proxies execution to a host-registered tool.
type HostTool struct {
	Name    string
	Execute func(ctx context.Context, args map[string]any) (*ToolResult, error)
}

// SessionInfo describes a session returned by Sessions().
type SessionInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CWD       string `json:"cwd"`
	CreatedAt string `json:"createdAt"` // RFC3339
	ParentID  string `json:"parentId,omitempty"`
	Path      string `json:"path"`
}

// ExtInfo describes a loaded extension returned by ExtInfos().
type ExtInfo struct {
	Name          string   `json:"name"`
	Version       string   `json:"version,omitempty"`
	Kind          string   `json:"kind"`
	Runtime       string   `json:"runtime,omitempty"`
	Tools         []string `json:"tools,omitempty"`
	Commands      []string `json:"commands,omitempty"`
	Interceptors  []string `json:"interceptors,omitempty"`
	EventHandlers []string `json:"eventHandlers,omitempty"`
	Shortcuts     []string `json:"shortcuts,omitempty"`
	MessageHooks  []string `json:"messageHooks,omitempty"`
	Compactor     string   `json:"compactor,omitempty"`
}

// ---------------------------------------------------------------------------
// Provider streaming types
// ---------------------------------------------------------------------------

// ProviderStreamRequest is the SDK-side type for provider/stream requests.
type ProviderStreamRequest struct {
	RequestID int             `json:"requestId"`
	Model     json.RawMessage `json:"model"`
	System    string          `json:"system"`
	Messages  json.RawMessage `json:"messages"`
	Tools     json.RawMessage `json:"tools,omitempty"`
	Options   json.RawMessage `json:"options,omitempty"`
}

// ProviderStreamResponse is the SDK-side result for provider/stream.
type ProviderStreamResponse struct {
	Message json.RawMessage `json:"message"`
	Error   string          `json:"error,omitempty"`
}

// ProviderDelta is the SDK-side notification for provider/delta.
type ProviderDelta struct {
	RequestID int               `json:"requestId"`
	Type      string            `json:"type"`
	Index     int               `json:"index,omitempty"`
	Delta     string            `json:"delta,omitempty"`
	Tool      *ProviderToolCall `json:"tool,omitempty"`
}

// ProviderToolCall is a tool call in a provider delta.
type ProviderToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
