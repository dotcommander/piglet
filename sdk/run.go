package sdk

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// ---------------------------------------------------------------------------
// Wire types (mirrors ext/external/protocol.go)
// ---------------------------------------------------------------------------

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type wireActionResult struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Run starts the JSON-RPC read loop. Blocks until the input pipe closes.
// When launched by piglet, reads from FD 3 (host→ext) and writes to FD 4 (ext→host).
// Falls back to stdin/stdout for manual debugging or non-piglet launch.
func (e *Extension) Run() {
	// Prefer FD 3/4 when host signals via PIGLET_FD=1
	var rpcIn *os.File
	if os.Getenv("PIGLET_FD") == "1" {
		rpcIn = os.NewFile(3, "piglet-rpc-in")
		e.rpcOut = os.NewFile(4, "piglet-rpc-out")
	} else {
		// Fallback to stdin/stdout (manual debugging, non-piglet launch)
		rpcIn = os.Stdin
		e.rpcOut = os.Stdout
	}

	scanner := bufio.NewScanner(rpcIn)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		e.handleMessage(&msg)
	}

	if err := scanner.Err(); err != nil {
		e.Log("error", fmt.Sprintf("read loop exited: %v", err))
	}
}

// ---------------------------------------------------------------------------
// Message handling
// ---------------------------------------------------------------------------

func (e *Extension) handleMessage(msg *rpcMessage) {
	// Handle notifications (no ID)
	if msg.ID == nil {
		switch msg.Method {
		case "$/cancelRequest":
			var p struct {
				ID int `json:"id"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			e.cancelMu.Lock()
			if cancel, ok := e.cancels[p.ID]; ok {
				cancel()
				delete(e.cancels, p.ID)
			}
			e.cancelMu.Unlock()
		case "eventBus/event":
			var p struct {
				SubscriptionID int             `json:"subscriptionId"`
				Data           json.RawMessage `json:"data"`
			}
			if json.Unmarshal(msg.Params, &p) == nil {
				e.subsMu.Lock()
				sub := e.subs[p.SubscriptionID]
				e.subsMu.Unlock()
				if sub != nil {
					select {
					case sub.ch <- p.Data:
					default: // drop if full
					}
				}
			}
		}
		return
	}

	// Response to an outgoing request (has ID, no method)
	if msg.Method == "" {
		e.pendingMu.Lock()
		ch, ok := e.pending[*msg.ID]
		if ok {
			delete(e.pending, *msg.ID)
		}
		e.pendingMu.Unlock()
		if ok {
			ch <- msg
		}
		return
	}

	// Requests from host — dispatched in goroutines so handlers can call
	// back to the host (e.g. CallHostTool) without deadlocking the read loop.
	switch msg.Method {
	case "initialize":
		// Async so OnInit callbacks can make host calls (e.g. ConfigReadExtension)
		// without deadlocking the read loop. Registrations still precede the
		// initialize response because handleInitialize sends them sequentially.
		go e.handleInitialize(msg)
	case "tool/execute":
		go e.handleToolExecute(msg)
	case "command/execute":
		go e.handleCommandExecute(msg)
	case "interceptor/before":
		go e.handleInterceptorBefore(msg)
	case "interceptor/after":
		go e.handleInterceptorAfter(msg)
	case "event/dispatch":
		go e.handleEventDispatch(msg)
	case "shortcut/handle":
		go e.handleShortcutHandle(msg)
	case "messageHook/onMessage":
		go e.handleMessageHook(msg)
	case "inputTransformer/transform":
		go e.handleInputTransform(msg)
	case "compact/execute":
		go e.handleCompactExecute(msg)
	case "provider/stream":
		go e.handleProviderStream(msg)
	case "shutdown":
		e.sendResponse(*msg.ID, nil)
		// Cancel all in-flight requests
		e.cancelMu.Lock()
		for id, cancel := range e.cancels {
			cancel()
			delete(e.cancels, id)
		}
		e.cancelMu.Unlock()
		os.Exit(0)
	default:
		e.sendError(*msg.ID, -32601, fmt.Sprintf("unknown method: %s", msg.Method))
	}
}

// ---------------------------------------------------------------------------
// Wire helpers
// ---------------------------------------------------------------------------

func (e *Extension) sendNotification(method string, params any) {
	data, _ := json.Marshal(params)
	e.write(&rpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  data,
	})
}

func (e *Extension) sendResponse(id int, result any) {
	data, _ := json.Marshal(result)
	e.write(&rpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  data,
	})
}

func (e *Extension) sendError(id int, code int, message string) {
	e.write(&rpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Error:   &rpcError{Code: code, Message: message},
	})
}

func (e *Extension) write(msg *rpcMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')

	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	_, _ = e.rpcOut.Write(data)
}
