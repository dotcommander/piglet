// Package sdk provides the Go Extension SDK for piglet.
//
// Extensions are standalone binaries that communicate with the piglet host
// via JSON-RPC over stdin/stdout. This SDK handles the protocol and provides
// a registration API mirroring the TypeScript SDK.
//
// Usage:
//
//	func main() {
//	    ext := sdk.New("my-extension", "0.1.0")
//	    ext.RegisterTool(sdk.ToolDef{...})
//	    ext.RegisterInterceptor(sdk.InterceptorDef{...})
//	    ext.Run()
//	}
package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Extension
// ---------------------------------------------------------------------------

// Extension manages the JSON-RPC lifecycle for a piglet extension.
type Extension struct {
	name      string
	version   string
	cwd       string
	configDir string // extension's namespaced config dir (from host init)

	tools          map[string]*ToolDef
	commands       map[string]*CommandDef
	promptSections []PromptSectionDef
	interceptors   []InterceptorDef
	eventHandlers  []EventHandlerDef
	shortcuts      map[string]*ShortcutDef
	messageHooks   []MessageHookDef
	compactor      *CompactorDef

	inputTransformers     []InputTransformerDef
	rpcOut                *os.File           // JSON-RPC output (FD 4 or os.Stdout fallback)
	onInit                func(e *Extension) // called after initialize, before responding
	providerStreamHandler func(ctx context.Context, x *Extension, req ProviderStreamRequest) (*ProviderStreamResponse, error)
	writeMu               sync.Mutex
	cancelMu              sync.Mutex
	cancels               map[int]context.CancelFunc // request ID → cancel

	// Outgoing request tracking (extension → host)
	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[int]chan *rpcMessage // request ID → response channel

	// Event bus subscriptions (host → extension notifications)
	subsMu sync.Mutex
	subs   map[int]*Subscription // subscription ID → Subscription
}

// New creates a new extension with the given name and version.
func New(name, version string) *Extension {
	return &Extension{
		name:      name,
		version:   version,
		rpcOut:    os.Stdout,
		tools:     make(map[string]*ToolDef),
		commands:  make(map[string]*CommandDef),
		shortcuts: make(map[string]*ShortcutDef),
		cancels:   make(map[int]context.CancelFunc),
		pending:   make(map[int]chan *rpcMessage),
		subs:      make(map[int]*Subscription),
	}
}

func (e *Extension) RegisterTool(t ToolDef) {
	if t.Name == "" {
		panic("sdk: RegisterTool called with empty Name")
	}
	if t.Execute == nil {
		panic("sdk: RegisterTool called with nil Execute function")
	}
	e.tools[t.Name] = &t
}

func (e *Extension) RegisterCommand(c CommandDef) {
	e.commands[c.Name] = &c
}

func (e *Extension) RegisterPromptSection(s PromptSectionDef) {
	e.promptSections = append(e.promptSections, s)
}

func (e *Extension) RegisterInterceptor(i InterceptorDef) {
	e.interceptors = append(e.interceptors, i)
}

func (e *Extension) RegisterEventHandler(h EventHandlerDef) {
	e.eventHandlers = append(e.eventHandlers, h)
}

func (e *Extension) RegisterShortcut(s ShortcutDef) {
	if s.Key == "" {
		panic("sdk: RegisterShortcut called with empty Key")
	}
	if s.Handler == nil {
		panic("sdk: RegisterShortcut called with nil Handler function")
	}
	e.shortcuts[s.Key] = &s
}

func (e *Extension) RegisterMessageHook(h MessageHookDef) {
	e.messageHooks = append(e.messageHooks, h)
}

func (e *Extension) RegisterCompactor(c CompactorDef) {
	e.compactor = &c
}

func (e *Extension) RegisterInputTransformer(t InputTransformerDef) {
	if t.Name == "" {
		panic("sdk: RegisterInputTransformer called with empty Name")
	}
	if t.Transform == nil {
		panic("sdk: RegisterInputTransformer called with nil Transform")
	}
	e.inputTransformers = append(e.inputTransformers, t)
}

// RegisterProvider declares that this extension provides LLM streaming for the given API type.
func (e *Extension) RegisterProvider(api string) {
	e.sendNotification("register/provider", map[string]string{"api": api})
}

// OnProviderStream registers a handler for provider/stream requests from the host.
func (e *Extension) OnProviderStream(handler func(ctx context.Context, x *Extension, req ProviderStreamRequest) (*ProviderStreamResponse, error)) {
	e.providerStreamHandler = handler
}

// SendProviderDelta sends a streaming delta notification to the host.
func (e *Extension) SendProviderDelta(requestID int, deltaType string, index int, delta string, tool *ProviderToolCall) {
	e.sendNotification("provider/delta", ProviderDelta{
		RequestID: requestID,
		Type:      deltaType,
		Index:     index,
		Delta:     delta,
		Tool:      tool,
	})
}

// OnInit sets a callback that runs after the host sends initialize (CWD is available)
// but before registrations are sent. Use this for lazy initialization that needs CWD.
func (e *Extension) OnInit(fn func(e *Extension)) {
	e.onInit = fn
}

// OnInitAppend chains an additional initialization function. Unlike OnInit which
// replaces the callback, OnInitAppend appends to the existing chain. Used by
// extension packs that compose multiple logical extensions into one binary.
func (e *Extension) OnInitAppend(fn func(e *Extension)) {
	prev := e.onInit
	if prev == nil {
		e.onInit = fn
		return
	}
	e.onInit = func(e *Extension) {
		prev(e)
		fn(e)
	}
}

// CWD returns the working directory provided by the host during initialization.
func (e *Extension) CWD() string { return e.cwd }

// ConfigDir returns the extension's namespaced config directory path,
// as provided by the host during initialization.
// Returns empty string if the host did not provide it.
func (e *Extension) ConfigDir() string { return e.configDir }

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

func (e *Extension) handleInitialize(msg *rpcMessage) {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
		CWD             string `json:"cwd"`
		ConfigDir       string `json:"configDir"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}
	e.cwd = params.CWD
	e.configDir = params.ConfigDir

	// Call OnInit hook (allows lazy registration that needs CWD)
	if e.onInit != nil {
		e.onInit(e)
	}

	// Send all registrations
	for _, t := range e.tools {
		params := map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
			"promptHint":  t.PromptHint,
			"deferred":    t.Deferred,
		}
		if t.InterruptBehavior != "" {
			params["interruptBehavior"] = t.InterruptBehavior
		}
		e.sendNotification("register/tool", params)
	}
	for _, c := range e.commands {
		params := map[string]any{
			"name":        c.Name,
			"description": c.Description,
		}
		if c.Immediate {
			params["immediate"] = true
		}
		e.sendNotification("register/command", params)
	}
	for _, s := range e.promptSections {
		params := map[string]any{
			"title":   s.Title,
			"content": s.Content,
			"order":   s.Order,
		}
		if s.TokenHint > 0 {
			params["tokenHint"] = s.TokenHint
		}
		e.sendNotification("register/promptSection", params)
	}
	for _, i := range e.interceptors {
		e.sendNotification("register/interceptor", map[string]any{
			"name":     i.Name,
			"priority": i.Priority,
		})
	}
	for _, h := range e.eventHandlers {
		e.sendNotification("register/eventHandler", map[string]any{
			"name":     h.Name,
			"priority": h.Priority,
			"events":   h.Events,
		})
	}
	for _, s := range e.shortcuts {
		e.sendNotification("register/shortcut", map[string]any{
			"key":         s.Key,
			"description": s.Description,
		})
	}
	for _, h := range e.messageHooks {
		e.sendNotification("register/messageHook", map[string]any{
			"name":     h.Name,
			"priority": h.Priority,
		})
	}
	for _, it := range e.inputTransformers {
		e.sendNotification("register/inputTransformer", map[string]any{
			"name":     it.Name,
			"priority": it.Priority,
		})
	}
	if e.compactor != nil {
		e.sendNotification("register/compactor", map[string]any{
			"name":      e.compactor.Name,
			"threshold": e.compactor.Threshold,
		})
	}

	// Respond
	e.sendResponse(*msg.ID, map[string]string{
		"name":    e.name,
		"version": e.version,
	})
}

func (e *Extension) handleToolExecute(msg *rpcMessage) {
	var params struct {
		CallID string         `json:"callId"`
		Name   string         `json:"name"`
		Args   map[string]any `json:"args"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	tool, ok := e.tools[params.Name]
	if !ok {
		e.sendError(*msg.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()
	result, err := tool.Execute(ctx, params.Args)
	if err != nil {
		e.sendError(*msg.ID, -32603, err.Error())
		return
	}

	e.sendResponse(*msg.ID, map[string]any{
		"content": result.Content,
		"isError": result.IsError,
	})
}

func (e *Extension) handleCommandExecute(msg *rpcMessage) {
	var params struct {
		Name string `json:"name"`
		Args string `json:"args"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	cmd, ok := e.commands[params.Name]
	if !ok {
		e.sendError(*msg.ID, -32602, fmt.Sprintf("unknown command: %s", params.Name))
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()
	if err := cmd.Handler(ctx, params.Args); err != nil {
		e.sendError(*msg.ID, -32603, err.Error())
		return
	}
	e.sendResponse(*msg.ID, map[string]any{})
}

func (e *Extension) handleInterceptorBefore(msg *rpcMessage) {
	var params struct {
		Name     string         `json:"name"`
		ToolName string         `json:"toolName"`
		Args     map[string]any `json:"args"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	// Run the targeted interceptor's Before hook (or all if name is empty).
	allow := true
	args := maps.Clone(params.Args)
	var preview string
	for _, ic := range e.interceptors {
		if ic.Before == nil {
			continue
		}
		if params.Name != "" && ic.Name != params.Name {
			continue
		}
		a, modified, err := ic.Before(ctx, params.ToolName, args)
		if err != nil {
			e.sendError(*msg.ID, -32603, err.Error())
			return
		}
		if !a {
			allow = false
			if ic.Preview != nil {
				preview = ic.Preview(ctx, params.ToolName, args)
			}
			break
		}
		if modified != nil {
			args = maps.Clone(modified)
		}
	}

	resp := map[string]any{
		"allow": allow,
		"args":  args,
	}
	if preview != "" {
		resp["preview"] = preview
	}
	e.sendResponse(*msg.ID, resp)
}

func (e *Extension) handleInterceptorAfter(msg *rpcMessage) {
	var params struct {
		Name     string `json:"name"`
		ToolName string `json:"toolName"`
		Details  any    `json:"details"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	details := params.Details
	for _, ic := range e.interceptors {
		if ic.After == nil {
			continue
		}
		if params.Name != "" && ic.Name != params.Name {
			continue
		}
		modified, err := ic.After(ctx, params.ToolName, details)
		if err != nil {
			e.Log("error", fmt.Sprintf("interceptor %q after hook: %v", ic.Name, err))
			continue
		}
		details = modified
	}

	e.sendResponse(*msg.ID, map[string]any{"details": details})
}

func (e *Extension) handleEventDispatch(msg *rpcMessage) {
	var params struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	var resultAction *wireActionResult
	for _, eh := range e.eventHandlers {
		if len(eh.Events) > 0 && !slices.Contains(eh.Events, params.Type) {
			continue
		}
		if eh.Handle == nil {
			continue
		}
		action := eh.Handle(ctx, params.Type, params.Data)
		if action != nil {
			payload, _ := json.Marshal(action.Payload)
			resultAction = &wireActionResult{Type: action.Type, Payload: payload}
			break // first action wins
		}
	}

	e.sendResponse(*msg.ID, map[string]any{"action": resultAction})
}

func (e *Extension) handleShortcutHandle(msg *rpcMessage) {
	var params struct {
		Key string `json:"key"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	// Match the specific shortcut by key
	sc, ok := e.shortcuts[params.Key]
	if !ok || sc.Handler == nil {
		e.sendResponse(*msg.ID, map[string]any{"action": nil})
		return
	}

	action, err := sc.Handler(ctx)
	if err != nil {
		e.sendError(*msg.ID, -32603, err.Error())
		return
	}
	var resultAction *wireActionResult
	if action != nil {
		payload, _ := json.Marshal(action.Payload)
		resultAction = &wireActionResult{Type: action.Type, Payload: payload}
	}

	e.sendResponse(*msg.ID, map[string]any{"action": resultAction})
}

func (e *Extension) handleMessageHook(msg *rpcMessage) {
	var params struct {
		Message string `json:"message"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	var injection string
	for _, mh := range e.messageHooks {
		if mh.OnMessage == nil {
			continue
		}
		extra, err := mh.OnMessage(ctx, params.Message)
		if err != nil {
			e.sendError(*msg.ID, -32603, err.Error())
			return
		}
		if extra != "" {
			if injection != "" {
				injection += "\n"
			}
			injection += extra
		}
	}

	e.sendResponse(*msg.ID, map[string]any{"injection": injection})
}

func (e *Extension) handleInputTransform(msg *rpcMessage) {
	var params struct {
		Input string `json:"input"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	current := params.Input
	for _, it := range e.inputTransformers {
		if it.Transform == nil {
			continue
		}
		output, handled, err := it.Transform(ctx, current)
		if err != nil {
			e.sendError(*msg.ID, -32603, err.Error())
			return
		}
		if handled {
			e.sendResponse(*msg.ID, map[string]any{"output": output, "handled": true})
			return
		}
		current = output
	}
	e.sendResponse(*msg.ID, map[string]any{"output": current, "handled": false})
}

func (e *Extension) handleCompactExecute(msg *rpcMessage) {
	if e.compactor == nil || e.compactor.Compact == nil {
		e.sendError(*msg.ID, -32603, "no compactor registered")
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	result, err := e.compactor.Compact(ctx, msg.Params)
	if err != nil {
		e.sendError(*msg.ID, -32603, err.Error())
		return
	}

	e.sendResponse(*msg.ID, result)
}

func (e *Extension) handleProviderStream(msg *rpcMessage) {
	if e.providerStreamHandler == nil {
		e.sendError(*msg.ID, -32603, "no provider stream handler registered")
		return
	}

	var req ProviderStreamRequest
	if !e.unmarshalParams(msg, &req) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	resp, err := e.providerStreamHandler(ctx, e, req)
	if err != nil {
		e.sendError(*msg.ID, -32603, err.Error())
		return
	}

	e.sendResponse(*msg.ID, resp)
}

// ---------------------------------------------------------------------------
// Request context management
// ---------------------------------------------------------------------------

// requestCtx creates a cancellable context for a request and tracks it for $/cancelRequest.
func (e *Extension) requestCtx(id int) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelMu.Lock()
	e.cancels[id] = cancel
	e.cancelMu.Unlock()
	cleanup := func() {
		cancel()
		e.cancelMu.Lock()
		delete(e.cancels, id)
		e.cancelMu.Unlock()
	}
	return ctx, cleanup
}

// unmarshalParams unmarshals msg.Params into v, returning false on error.
func (e *Extension) unmarshalParams(msg *rpcMessage, v any) bool {
	if err := json.Unmarshal(msg.Params, v); err != nil {
		e.sendError(*msg.ID, -32600, fmt.Sprintf("invalid params: %v", err))
		return false
	}
	return true
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
