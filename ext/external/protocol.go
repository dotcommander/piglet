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

// RegisterCompactorParams registers a conversation compactor.
type RegisterCompactorParams struct {
	Name      string `json:"name"`
	Threshold int    `json:"threshold,omitempty"` // 0 = use config default
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

// SendMessageParams is the payload for a sendMessage notification.
type SendMessageParams struct {
	Content string `json:"content"`
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
const ProtocolVersion = "3"

// CancelParams tells the extension to abort the request with the given ID.
type CancelParams struct {
	ID int `json:"id"`
}

// ---------------------------------------------------------------------------
// Host config service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostConfigGetParams requests configuration values.
type HostConfigGetParams struct {
	Keys []string `json:"keys"`
}

// HostConfigGetResult is the host's response.
type HostConfigGetResult struct {
	Values map[string]any `json:"values"`
}

// HostConfigReadExtParams requests an extension's markdown config.
type HostConfigReadExtParams struct {
	Name string `json:"name"`
}

// HostConfigReadExtResult is the host's response.
type HostConfigReadExtResult struct {
	Content string `json:"content"`
}

// ---------------------------------------------------------------------------
// Host auth service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostAuthGetKeyParams requests an API key for a provider.
type HostAuthGetKeyParams struct {
	Provider string `json:"provider"`
}

// HostAuthGetKeyResult is the host's response.
type HostAuthGetKeyResult struct {
	Key string `json:"key"`
}

// ---------------------------------------------------------------------------
// Host chat service: extension → host (request/response)
// ---------------------------------------------------------------------------

// ChatMessage is a single message in a chat request.
type ChatMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"`
}

// HostChatParams asks the host to make a single-turn LLM call.
type HostChatParams struct {
	System    string        `json:"system,omitempty"`
	Messages  []ChatMessage `json:"messages"`
	Model     string        `json:"model,omitempty"`     // "small", "default", or explicit model ID
	MaxTokens int           `json:"maxTokens,omitempty"`
}

// HostChatResult is the host's response.
type HostChatResult struct {
	Text  string         `json:"text"`
	Usage HostTokenUsage `json:"usage"`
}

// HostTokenUsage reports token consumption.
type HostTokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

// ---------------------------------------------------------------------------
// Host agent service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostAgentRunParams asks the host to run a full agent loop.
type HostAgentRunParams struct {
	System   string `json:"system,omitempty"`
	Task     string `json:"task"`
	Tools    string `json:"tools,omitempty"`    // "background_safe" (default) or "all"
	Model    string `json:"model,omitempty"`    // "small", "default", or explicit model ID
	MaxTurns int    `json:"maxTurns,omitempty"`
}

// HostAgentRunResult is the host's response.
type HostAgentRunResult struct {
	Text  string         `json:"text"`
	Turns int            `json:"turns"`
	Usage HostTokenUsage `json:"usage"`
}

// ---------------------------------------------------------------------------
// Host session service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostConversationMessagesResult is the host's response with raw message JSON.
type HostConversationMessagesResult struct {
	Messages json.RawMessage `json:"messages"`
}

// HostSessionsResult is the host's response with session summaries.
type HostSessionsResult struct {
	Sessions []WireSessionInfo `json:"sessions"`
}

// WireSessionInfo is the wire representation of a session summary.
type WireSessionInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CWD       string `json:"cwd"`
	CreatedAt string `json:"createdAt"` // RFC3339
	ParentID  string `json:"parentId,omitempty"`
	Path      string `json:"path"`
}

// HostLoadSessionParams requests loading a session by path.
type HostLoadSessionParams struct {
	Path string `json:"path"`
}

// HostForkSessionResult is the host's response after forking.
type HostForkSessionResult struct {
	ParentID     string `json:"parentID"`
	MessageCount int    `json:"messageCount"`
}

// HostSetSessionTitleParams sets the current session's title.
type HostSetSessionTitleParams struct {
	Title string `json:"title"`
}

// HostSyncModelsResult is the host's response after syncing models.
type HostSyncModelsResult struct {
	Updated int `json:"updated"`
}

// HostRunBackgroundParams starts a background agent.
type HostRunBackgroundParams struct {
	Prompt string `json:"prompt"`
}

// HostIsBackgroundRunningResult is the host's response.
type HostIsBackgroundRunningResult struct {
	Running bool `json:"running"`
}

// ---------------------------------------------------------------------------
// Host extension info service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostExtInfosResult is the host's response with loaded extension metadata.
type HostExtInfosResult struct {
	Extensions []WireExtInfo `json:"extensions"`
}

// WireExtInfo is the wire representation of an extension's metadata.
type WireExtInfo struct {
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

// HostExtensionsDirResult is the host's response with the extensions directory.
type HostExtensionsDirResult struct {
	Path string `json:"path"`
}

// ---------------------------------------------------------------------------
// Host undo service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostUndoSnapshotsResult is the host's response with undo snapshot info.
type HostUndoSnapshotsResult struct {
	Snapshots map[string]int `json:"snapshots"` // path → size in bytes
}

// HostUndoRestoreParams requests restoring a file from its undo snapshot.
type HostUndoRestoreParams struct {
	Path string `json:"path"`
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
	MethodRegisterCompactor     = "register/compactor"
	MethodCompactExecute        = "compact/execute"
	MethodToolExecute           = "tool/execute"
	MethodCommandExecute        = "command/execute"
	MethodInterceptorBefore     = "interceptor/before"
	MethodInterceptorAfter      = "interceptor/after"
	MethodEventDispatch         = "event/dispatch"
	MethodShortcutHandle        = "shortcut/handle"
	MethodMessageHookOnMessage  = "messageHook/onMessage"
	MethodHostListTools              = "host/listTools"
	MethodHostExecuteTool            = "host/executeTool"
	MethodHostConfigGet              = "host/config.get"
	MethodHostConfigReadExt          = "host/config.readExtension"
	MethodHostAuthGetKey             = "host/auth.getKey"
	MethodHostChat                   = "host/chat"
	MethodHostAgentRun               = "host/agent.run"
	MethodHostConversationMessages   = "host/conversationMessages"
	MethodHostSessions               = "host/sessions"
	MethodHostLoadSession            = "host/loadSession"
	MethodHostForkSession            = "host/forkSession"
	MethodHostSetSessionTitle        = "host/setSessionTitle"
	MethodHostSyncModels             = "host/syncModels"
	MethodHostRunBackground          = "host/runBackground"
	MethodHostCancelBackground       = "host/cancelBackground"
	MethodHostIsBackgroundRunning    = "host/isBackgroundRunning"
	MethodHostExtInfos               = "host/extInfos"
	MethodHostExtensionsDir          = "host/extensionsDir"
	MethodHostUndoSnapshots          = "host/undoSnapshots"
	MethodHostUndoRestore            = "host/undoRestore"
	MethodNotify                     = "notify"
	MethodLog                   = "log"
	MethodShowMessage           = "showMessage"
	MethodSendMessage           = "sendMessage"
)
