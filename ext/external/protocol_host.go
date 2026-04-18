package external

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
	Deferred    bool   `json:"deferred,omitempty"`
}

// HostExecuteToolParams is sent by the extension to execute a host-registered tool.
type HostExecuteToolParams struct {
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
	CallID string         `json:"callId,omitzero"`
}

// HostExecuteToolResult is the host's response.
type HostExecuteToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
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
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// HostChatParams asks the host to make a single-turn LLM call.
type HostChatParams struct {
	System    string        `json:"system,omitempty"`
	Messages  []ChatMessage `json:"messages"`
	Model     string        `json:"model,omitempty"` // "small", "default", or explicit model ID
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
	Tools    string `json:"tools,omitempty"` // "background_safe" (default) or "all"
	Model    string `json:"model,omitempty"` // "small", "default", or explicit model ID
	MaxTurns int    `json:"maxTurns,omitempty"`
}

// HostAgentRunResult is the host's response.
type HostAgentRunResult struct {
	Text  string         `json:"text"`
	Turns int            `json:"turns"`
	Usage HostTokenUsage `json:"usage"`
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

// ---------------------------------------------------------------------------
// Host last assistant text: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostLastAssistantTextResult is the host's response.
type HostLastAssistantTextResult struct {
	Text string `json:"text"`
}
