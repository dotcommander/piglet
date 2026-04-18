// Package external implements the JSON-RPC stdio protocol for external
// piglet extensions. Extensions are child processes that communicate
// via newline-delimited JSON on stdin/stdout.
package external

import "encoding/json"

// InterruptBehaviorBlock is the wire value for InterruptBlock on external tools.
const InterruptBehaviorBlock = "block"

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
	ConfigDir       string `json:"configDir,omitempty"`
}

// InitializeResult is the extension's response to initialize.
type InitializeResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ---------------------------------------------------------------------------
// Lifecycle: host → extension
// ---------------------------------------------------------------------------

// ShutdownParams asks the extension to shut down gracefully.
type ShutdownParams struct{}

// Protocol version
const ProtocolVersion = "5"

// CancelParams tells the extension to abort the request with the given ID.
type CancelParams struct {
	ID int `json:"id"`
}
