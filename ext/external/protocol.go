// Package external implements the JSON-RPC stdio protocol for external
// piglet extensions. Extensions are child processes that communicate
// via newline-delimited JSON on stdin/stdout.
package external

import "encoding/json"

// ---------------------------------------------------------------------------
// Wire format
// ---------------------------------------------------------------------------

// Message is a JSON-RPC 2.0-ish envelope. We use a simplified variant:
// requests have Method+ID+Params, responses have ID+Result or ID+Error,
// notifications have Method+Params and no ID.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is the error object in a JSON-RPC response.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Handshake: host → extension
// ---------------------------------------------------------------------------

// InitializeParams is sent by the host after spawning the extension.
type InitializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
	CWD             string `json:"cwd"`
}

// InitializeResult is the extension's response to initialize.
type InitializeResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ---------------------------------------------------------------------------
// Registration: extension → host (notifications, no response expected)
// ---------------------------------------------------------------------------

// RegisterToolParams registers a tool the LLM can call.
type RegisterToolParams struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema
	PromptHint  string `json:"promptHint,omitempty"`
}

// RegisterCommandParams registers a slash command.
type RegisterCommandParams struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RegisterPromptSectionParams adds a section to the system prompt.
type RegisterPromptSectionParams struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Order   int    `json:"order,omitempty"`
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
	Key         string `json:"key"`         // e.g. "ctrl+g"
	Description string `json:"description"`
}

// RegisterMessageHookParams registers a pre-message hook.
type RegisterMessageHookParams struct {
	Name     string `json:"name"`
	Priority int    `json:"priority,omitempty"`
}

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
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
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
	ToolName string         `json:"toolName"`
	Args     map[string]any `json:"args"`
}

// InterceptorBeforeResult is the extension's response.
type InterceptorBeforeResult struct {
	Allow bool           `json:"allow"`
	Args  map[string]any `json:"args,omitempty"`
}

// InterceptorAfterParams is sent after tool execution.
type InterceptorAfterParams struct {
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
// Serializable action (replaces Go Action interface over the wire)
// ---------------------------------------------------------------------------

// ActionResult is a JSON-serializable action returned by extensions.
// The host converts it to the appropriate ext.Action type.
type ActionResult struct {
	Type    string          `json:"type"`              // "notify", "showMessage", "setSessionTitle", "quit"
	Payload json.RawMessage `json:"payload,omitempty"` // type-specific data
}

// ---------------------------------------------------------------------------
// Host tool execution: extension → host (request/response)
// ---------------------------------------------------------------------------

// Tool filter constants for HostListToolsParams.
const (
	FilterAll            = "all"
	FilterBackgroundSafe = "background_safe"
)

// HostListToolsParams requests the list of available host tools.
type HostListToolsParams struct {
	Filter string `json:"filter,omitempty"` // FilterAll or FilterBackgroundSafe (default: FilterAll)
}

// HostListToolsResult is the host's response.
type HostListToolsResult struct {
	Tools []HostToolInfo `json:"tools"`
}

// HostToolInfo describes a single host-registered tool.
type HostToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema
}

// HostExecuteToolParams is sent by the extension to execute a host-registered tool.
type HostExecuteToolParams struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// HostExecuteToolResult is the host's response.
type HostExecuteToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ---------------------------------------------------------------------------
// Lifecycle: host → extension
// ---------------------------------------------------------------------------

// ShutdownParams asks the extension to shut down gracefully.
type ShutdownParams struct{}

// Protocol version
const ProtocolVersion = "2"

// CancelParams tells the extension to abort the request with the given ID.
type CancelParams struct {
	ID int `json:"id"`
}

// Method names
const (
	MethodInitialize            = "initialize"
	MethodShutdown              = "shutdown"
	MethodCancelRequest         = "$/cancelRequest"
	MethodRegisterTool          = "register/tool"
	MethodRegisterCommand       = "register/command"
	MethodRegisterPromptSection = "register/promptSection"
	MethodRegisterInterceptor   = "register/interceptor"
	MethodRegisterEventHandler  = "register/eventHandler"
	MethodRegisterShortcut      = "register/shortcut"
	MethodRegisterMessageHook   = "register/messageHook"
	MethodToolExecute           = "tool/execute"
	MethodCommandExecute        = "command/execute"
	MethodInterceptorBefore     = "interceptor/before"
	MethodInterceptorAfter      = "interceptor/after"
	MethodEventDispatch         = "event/dispatch"
	MethodShortcutHandle        = "shortcut/handle"
	MethodMessageHookOnMessage  = "messageHook/onMessage"
	MethodHostListTools         = "host/listTools"
	MethodHostExecuteTool       = "host/executeTool"
	MethodNotify                = "notify"
	MethodLog                   = "log"
	MethodShowMessage           = "showMessage"
)
