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
	"os"
	"slices"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// ToolDef defines a tool the LLM can call.
type ToolDef struct {
	Name        string
	Description string
	Parameters  any // JSON Schema
	PromptHint  string
	Execute     func(ctx context.Context, args map[string]any) (*ToolResult, error)
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
	Handler     func(ctx context.Context, args string) error
}

// PromptSectionDef defines a system prompt section.
type PromptSectionDef struct {
	Title   string
	Content string
	Order   int
}

// InterceptorDef defines a before/after tool interceptor.
type InterceptorDef struct {
	Name     string
	Priority int
	Before   func(ctx context.Context, toolName string, args map[string]any) (allow bool, modified map[string]any, err error)
	After    func(ctx context.Context, toolName string, details any) (any, error)
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

func ActionQuit() *Action {
	return &Action{Type: "quit"}
}

func ActionAttachImage(data, mimeType string) *Action {
	return &Action{Type: "attachImage", Payload: map[string]string{"data": data, "mimeType": mimeType}}
}

func ActionDetachImage() *Action {
	return &Action{Type: "detachImage"}
}

// ---------------------------------------------------------------------------
// Extension
// ---------------------------------------------------------------------------

// Extension manages the JSON-RPC lifecycle for a piglet extension.
type Extension struct {
	name    string
	version string
	cwd     string

	tools          map[string]*ToolDef
	commands       map[string]*CommandDef
	promptSections []PromptSectionDef
	interceptors   []InterceptorDef
	eventHandlers  []EventHandlerDef
	shortcuts      map[string]*ShortcutDef
	messageHooks   []MessageHookDef
	compactor      *CompactorDef

	onInit   func(e *Extension) // called after initialize, before responding
	writeMu  sync.Mutex
	cancelMu sync.Mutex
	cancels  map[int]context.CancelFunc // request ID → cancel

	// Outgoing request tracking (extension → host)
	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[int]chan *rpcMessage // request ID → response channel
}

// New creates a new extension with the given name and version.
func New(name, version string) *Extension {
	return &Extension{
		name:      name,
		version:   version,
		tools:     make(map[string]*ToolDef),
		commands:  make(map[string]*CommandDef),
		shortcuts: make(map[string]*ShortcutDef),
		cancels:   make(map[int]context.CancelFunc),
		pending:   make(map[int]chan *rpcMessage),
	}
}

func (e *Extension) RegisterTool(t ToolDef) {
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
	e.shortcuts[s.Key] = &s
}

func (e *Extension) RegisterMessageHook(h MessageHookDef) {
	e.messageHooks = append(e.messageHooks, h)
}

func (e *Extension) RegisterCompactor(c CompactorDef) {
	e.compactor = &c
}

// OnInit sets a callback that runs after the host sends initialize (CWD is available)
// but before registrations are sent. Use this for lazy initialization that needs CWD.
func (e *Extension) OnInit(fn func(e *Extension)) {
	e.onInit = fn
}

// CWD returns the working directory provided by the host during initialization.
func (e *Extension) CWD() string { return e.cwd }

// Notify sends a notification to the host TUI.
func (e *Extension) Notify(msg string) {
	e.sendNotification("notify", map[string]string{"message": msg})
}

// ShowMessage displays a message in the conversation.
func (e *Extension) ShowMessage(text string) {
	e.sendNotification("showMessage", map[string]string{"text": text})
}

// Log sends a log message to the host.
func (e *Extension) Log(level, msg string) {
	e.sendNotification("log", map[string]string{"level": level, "message": msg})
}

// ListHostTools returns the schemas of tools available in the host.
// Filter can be "all" (default) or "background_safe".
func (e *Extension) ListHostTools(ctx context.Context, filter string) ([]HostToolInfo, error) {
	resp, err := e.request(ctx, "host/listTools", map[string]any{"filter": filter})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("list host tools: %s", resp.Error.Message)
	}

	var result struct {
		Tools []HostToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal host tools: %w", err)
	}
	return result.Tools, nil
}

// HostToolInfo describes a host-registered tool.
type HostToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// CallHostTool executes a host-registered tool and returns the result.
// This allows extensions to use tools like Read, Edit, Grep, Bash that are
// registered in the host process. The call blocks until the host responds.
func (e *Extension) CallHostTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	resp, err := e.request(ctx, "host/executeTool", map[string]any{
		"name": name,
		"args": args,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("host tool %s: %s", name, resp.Error.Message)
	}

	var result struct {
		Content []wireContentBlock `json:"content"`
		IsError bool               `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal host tool result: %w", err)
	}

	blocks := make([]ContentBlock, len(result.Content))
	for i, b := range result.Content {
		blocks[i] = ContentBlock(b)
	}
	return &ToolResult{Content: blocks, IsError: result.IsError}, nil
}

// HostTools returns core.Tool wrappers that proxy tool calls to the host.
// Use this to give a sub-agent access to host-registered tools.
func (e *Extension) HostTools(names ...string) []HostTool {
	tools := make([]HostTool, len(names))
	for i, name := range names {
		name := name
		tools[i] = HostTool{
			Name: name,
			Execute: func(ctx context.Context, args map[string]any) (*ToolResult, error) {
				return e.CallHostTool(ctx, name, args)
			},
		}
	}
	return tools
}

// HostTool is a thin tool wrapper that proxies execution to a host-registered tool.
type HostTool struct {
	Name    string
	Execute func(ctx context.Context, args map[string]any) (*ToolResult, error)
}

// ---------------------------------------------------------------------------
// Host service types (extension → host requests)
// ---------------------------------------------------------------------------

// ChatMessage is a single message in a chat request.
type ChatMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
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

// ---------------------------------------------------------------------------
// Host service methods (extension → host)
// ---------------------------------------------------------------------------

// ConfigGet retrieves configuration values from the host.
// Keys use dot notation (e.g. "defaultModel", "agent.compactAt").
// Returns a map of key → value. Missing keys are omitted.
func (e *Extension) ConfigGet(ctx context.Context, keys ...string) (map[string]any, error) {
	resp, err := e.request(ctx, "host/config.get", map[string]any{"keys": keys})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("config get: %s", resp.Error.Message)
	}
	var result struct {
		Values map[string]any `json:"values"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return result.Values, nil
}

// ConfigReadExtension reads an extension's markdown config file from
// ~/.config/piglet/<name>.md via the host.
func (e *Extension) ConfigReadExtension(ctx context.Context, name string) (string, error) {
	resp, err := e.request(ctx, "host/config.readExtension", map[string]any{"name": name})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("config read extension: %s", resp.Error.Message)
	}
	var result struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("unmarshal extension config: %w", err)
	}
	return result.Content, nil
}

// AuthGetKey retrieves an API key for a provider from the host's auth store.
func (e *Extension) AuthGetKey(ctx context.Context, provider string) (string, error) {
	resp, err := e.request(ctx, "host/auth.getKey", map[string]any{"provider": provider})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("auth get key: %s", resp.Error.Message)
	}
	var result struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("unmarshal auth key: %w", err)
	}
	return result.Key, nil
}

// Chat makes a single-turn LLM call via the host. The host handles model
// resolution, provider creation, and streaming. Use for lightweight calls
// like title generation or summary refinement.
func (e *Extension) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	resp, err := e.request(ctx, "host/chat", req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("chat: %s", resp.Error.Message)
	}
	var result ChatResponse
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal chat response: %w", err)
	}
	return &result, nil
}

// RunAgent asks the host to run a full agent loop with tools to completion.
// The host handles model resolution, tool access, and multi-turn execution.
// Returns the final agent text output and usage statistics.
func (e *Extension) RunAgent(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
	resp, err := e.request(ctx, "host/agent.run", req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("run agent: %s", resp.Error.Message)
	}
	var result AgentResponse
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal agent response: %w", err)
	}
	return &result, nil
}

// request sends a JSON-RPC request to the host and waits for the response.
func (e *Extension) request(ctx context.Context, method string, params any) (*rpcMessage, error) {
	id := int(e.nextID.Add(1))

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	ch := make(chan *rpcMessage, 1)
	e.pendingMu.Lock()
	e.pending[id] = ch
	e.pendingMu.Unlock()

	e.write(&rpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  paramsJSON,
	})

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		e.pendingMu.Lock()
		delete(e.pending, id)
		e.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

// Run starts the JSON-RPC read loop. Blocks until stdin closes.
func (e *Extension) Run() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

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

type wireContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"`
	Mime string `json:"mime,omitempty"`
}

type wireActionResult struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ---------------------------------------------------------------------------
// Message handling
// ---------------------------------------------------------------------------

func (e *Extension) handleMessage(msg *rpcMessage) {
	// Handle notifications (no ID) — currently only $/cancelRequest
	if msg.ID == nil {
		if msg.Method == "$/cancelRequest" {
			var p struct{ ID int `json:"id"` }
			_ = json.Unmarshal(msg.Params, &p)
			e.cancelMu.Lock()
			if cancel, ok := e.cancels[p.ID]; ok {
				cancel()
				delete(e.cancels, p.ID)
			}
			e.cancelMu.Unlock()
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
		// Initialize must be synchronous (registrations happen before response)
		e.handleInitialize(msg)
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
	case "compact/execute":
		go e.handleCompactExecute(msg)
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
	}
	_ = json.Unmarshal(msg.Params, &params)
	e.cwd = params.CWD

	// Call OnInit hook (allows lazy registration that needs CWD)
	if e.onInit != nil {
		e.onInit(e)
	}

	// Send all registrations
	for _, t := range e.tools {
		e.sendNotification("register/tool", map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
			"promptHint":  t.PromptHint,
		})
	}
	for _, c := range e.commands {
		e.sendNotification("register/command", map[string]any{
			"name":        c.Name,
			"description": c.Description,
		})
	}
	for _, s := range e.promptSections {
		e.sendNotification("register/promptSection", map[string]any{
			"title":   s.Title,
			"content": s.Content,
			"order":   s.Order,
		})
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
	_ = json.Unmarshal(msg.Params, &params)

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

	blocks := make([]wireContentBlock, len(result.Content))
	for i, b := range result.Content {
		blocks[i] = wireContentBlock(b)
	}
	e.sendResponse(*msg.ID, map[string]any{
		"content": blocks,
		"isError": result.IsError,
	})
}

func (e *Extension) handleCommandExecute(msg *rpcMessage) {
	var params struct {
		Name string `json:"name"`
		Args string `json:"args"`
	}
	_ = json.Unmarshal(msg.Params, &params)

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
		ToolName string         `json:"toolName"`
		Args     map[string]any `json:"args"`
	}
	_ = json.Unmarshal(msg.Params, &params)

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	// Run all interceptors' Before hooks in order
	allow := true
	args := params.Args
	for _, ic := range e.interceptors {
		if ic.Before == nil {
			continue
		}
		a, modified, err := ic.Before(ctx, params.ToolName, args)
		if err != nil {
			e.sendError(*msg.ID, -32603, err.Error())
			return
		}
		if !a {
			allow = false
			break
		}
		if modified != nil {
			args = modified
		}
	}

	e.sendResponse(*msg.ID, map[string]any{
		"allow": allow,
		"args":  args,
	})
}

func (e *Extension) handleInterceptorAfter(msg *rpcMessage) {
	var params struct {
		ToolName string `json:"toolName"`
		Details  any    `json:"details"`
	}
	_ = json.Unmarshal(msg.Params, &params)

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	details := params.Details
	for _, ic := range e.interceptors {
		if ic.After == nil {
			continue
		}
		modified, err := ic.After(ctx, params.ToolName, details)
		if err != nil {
			continue // log but don't fail
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
	_ = json.Unmarshal(msg.Params, &params)

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
	_ = json.Unmarshal(msg.Params, &params)

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
	_ = json.Unmarshal(msg.Params, &params)

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
	_, _ = os.Stdout.Write(data)
}
