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
// Lifecycle: host → extension
// ---------------------------------------------------------------------------

// ShutdownParams asks the extension to shut down gracefully.
type ShutdownParams struct{}

// Protocol version
const ProtocolVersion = "1"

// Method names
const (
	MethodInitialize           = "initialize"
	MethodShutdown             = "shutdown"
	MethodRegisterTool         = "register/tool"
	MethodRegisterCommand      = "register/command"
	MethodRegisterPromptSection = "register/promptSection"
	MethodToolExecute          = "tool/execute"
	MethodCommandExecute       = "command/execute"
	MethodNotify               = "notify"
	MethodLog                  = "log"
	MethodShowMessage          = "showMessage"
)
