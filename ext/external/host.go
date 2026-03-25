package external

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/tool"
)

// Host manages a single external extension process.
type Host struct {
	manifest *Manifest
	cwd      string
	app      *ext.App // nil until bridge wires it

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner

	writeMu   sync.Mutex           // protects stdin writes
	pendingMu sync.Mutex           // protects pending map
	nextID    atomic.Int64
	pending   map[int]chan *Message // pending request ID → response channel
	closed    chan struct{}
	closeOnce sync.Once

	// Registrations collected during handshake
	tools          []RegisterToolParams
	commands       []RegisterCommandParams
	promptSections []RegisterPromptSectionParams
	interceptors   []RegisterInterceptorParams
	eventHandlers  []RegisterEventHandlerParams
	shortcuts      []RegisterShortcutParams
	messageHooks   []RegisterMessageHookParams
	compactor      *RegisterCompactorParams
}

// NewHost creates a host for the given manifest.
func NewHost(m *Manifest, cwd string) *Host {
	return &Host{
		manifest: m,
		cwd:      cwd,
		pending:  make(map[int]chan *Message),
		closed:   make(chan struct{}),
	}
}

// SetApp wires the host to the main application for runtime notifications.
func (h *Host) SetApp(app *ext.App) { h.app = app }

// Start spawns the extension process, performs the initialize handshake,
// and collects registrations. Returns after the extension sends initialize result.
func (h *Host) Start(ctx context.Context) error {
	bin, args := h.manifest.RuntimeCommand()
	h.cmd = exec.CommandContext(ctx, bin, args...)
	h.cmd.Dir = h.manifest.Dir

	var err error
	h.stdin, err = h.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := h.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	h.stdout = bufio.NewScanner(stdoutPipe)
	h.stdout.Buffer(make([]byte, 0, 256*1024), 1024*1024) // 1MB max line

	// Capture stderr for logging
	h.cmd.Stderr = &logWriter{name: h.manifest.Name}

	if err := h.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", h.manifest.Name, err)
	}

	// Start reading messages in background
	go h.readLoop()

	// Send initialize with a 10-second timeout to prevent hanging
	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	defer initCancel()

	result, err := h.request(initCtx, MethodInitialize, InitializeParams{
		ProtocolVersion: ProtocolVersion,
		CWD:             h.cwd,
	})
	if err != nil {
		h.Stop()
		return fmt.Errorf("initialize %s: %w", h.manifest.Name, err)
	}

	var initResult InitializeResult
	if result.Result != nil {
		_ = json.Unmarshal(result.Result, &initResult)
	}

	slog.Debug("extension initialized", "name", h.manifest.Name, "ext_version", initResult.Version)
	return nil
}

// Stop sends shutdown and kills the process.
func (h *Host) Stop() {
	h.closeOnce.Do(func() {
		// Best-effort shutdown notification
		_ = h.send(&Message{
			JSONRPC: "2.0",
			Method:  MethodShutdown,
		})

		_ = h.stdin.Close()
		close(h.closed)

		if h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
		_ = h.cmd.Wait()
	})
}

// Name returns the extension name from the manifest.
func (h *Host) Name() string { return h.manifest.Name }

// Tools returns the tools registered during handshake.
func (h *Host) Tools() []RegisterToolParams { return h.tools }

// Commands returns the commands registered during handshake.
func (h *Host) Commands() []RegisterCommandParams { return h.commands }

// PromptSections returns the prompt sections registered during handshake.
func (h *Host) PromptSections() []RegisterPromptSectionParams { return h.promptSections }

// Interceptors returns the interceptors registered during handshake.
func (h *Host) Interceptors() []RegisterInterceptorParams { return h.interceptors }

// EventHandlers returns the event handlers registered during handshake.
func (h *Host) EventHandlers() []RegisterEventHandlerParams { return h.eventHandlers }

// Shortcuts returns the shortcuts registered during handshake.
func (h *Host) Shortcuts() []RegisterShortcutParams { return h.shortcuts }

// MessageHooks returns the message hooks registered during handshake.
func (h *Host) MessageHooks() []RegisterMessageHookParams { return h.messageHooks }

// Compactor returns the compactor registered during handshake, or nil.
func (h *Host) Compactor() *RegisterCompactorParams { return h.compactor }

// ExecuteTool sends a tool/execute request and waits for the response.
func (h *Host) ExecuteTool(ctx context.Context, callID, name string, args map[string]any) (*ToolExecuteResult, error) {
	resp, err := h.request(ctx, MethodToolExecute, ToolExecuteParams{
		CallID: callID,
		Name:   name,
		Args:   args,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tool %s: %s", name, resp.Error.Message)
	}

	var result ToolExecuteResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tool result: %w", err)
	}
	return &result, nil
}

// ExecuteCommand sends a command/execute request and waits for the response.
func (h *Host) ExecuteCommand(ctx context.Context, name, args string) error {
	resp, err := h.request(ctx, MethodCommandExecute, CommandExecuteParams{
		Name: name,
		Args: args,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("command %s: %s", name, resp.Error.Message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Callbacks: host → extension
// ---------------------------------------------------------------------------

// InterceptBefore sends an interceptor/before request to the extension.
func (h *Host) InterceptBefore(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
	resp, err := h.request(ctx, MethodInterceptorBefore, InterceptorBeforeParams{
		ToolName: toolName,
		Args:     args,
	})
	if err != nil {
		return true, args, err // allow on error to avoid blocking
	}
	if resp.Error != nil {
		return true, args, fmt.Errorf("interceptor before: %s", resp.Error.Message)
	}
	var result InterceptorBeforeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return true, args, fmt.Errorf("unmarshal interceptor before: %w", err)
	}
	if result.Args != nil {
		return result.Allow, result.Args, nil
	}
	return result.Allow, args, nil
}

// InterceptAfter sends an interceptor/after request to the extension.
func (h *Host) InterceptAfter(ctx context.Context, toolName string, details any) (any, error) {
	resp, err := h.request(ctx, MethodInterceptorAfter, InterceptorAfterParams{
		ToolName: toolName,
		Details:  details,
	})
	if err != nil {
		return details, err
	}
	if resp.Error != nil {
		return details, fmt.Errorf("interceptor after: %s", resp.Error.Message)
	}
	var result InterceptorAfterResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return details, fmt.Errorf("unmarshal interceptor after: %w", err)
	}
	return result.Details, nil
}

// DispatchEvent sends an event/dispatch request to the extension.
func (h *Host) DispatchEvent(ctx context.Context, eventType string, data json.RawMessage) (*ActionResult, error) {
	resp, err := h.request(ctx, MethodEventDispatch, EventDispatchParams{
		Type: eventType,
		Data: data,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("event dispatch: %s", resp.Error.Message)
	}
	var result EventDispatchResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal event dispatch: %w", err)
	}
	return result.Action, nil
}

// HandleShortcut sends a shortcut/handle request to the extension.
func (h *Host) HandleShortcut(ctx context.Context, key string) (*ActionResult, error) {
	resp, err := h.request(ctx, MethodShortcutHandle, ShortcutHandleParams{Key: key})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("shortcut handle: %s", resp.Error.Message)
	}
	var result ShortcutHandleResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal shortcut handle: %w", err)
	}
	return result.Action, nil
}

// OnMessage sends a messageHook/onMessage request to the extension.
func (h *Host) OnMessage(ctx context.Context, msg string) (string, error) {
	resp, err := h.request(ctx, MethodMessageHookOnMessage, MessageHookParams{
		Message: msg,
	})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("message hook: %s", resp.Error.Message)
	}
	var result MessageHookResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("unmarshal message hook: %w", err)
	}
	return result.Injection, nil
}

// ExecuteCompact sends a compact/execute request with messages and waits for compacted result.
func (h *Host) ExecuteCompact(ctx context.Context, messages []CompactMessage) ([]CompactMessage, error) {
	resp, err := h.request(ctx, MethodCompactExecute, CompactExecuteParams{
		Messages: messages,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("compact execute: %s", resp.Error.Message)
	}
	var result CompactExecuteResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal compact result: %w", err)
	}
	return result.Messages, nil
}

// ---------------------------------------------------------------------------
// Internal
// ---------------------------------------------------------------------------

func (h *Host) request(ctx context.Context, method string, params any) (*Message, error) {
	id := int(h.nextID.Add(1))

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	ch := make(chan *Message, 1)
	h.pendingMu.Lock()
	h.pending[id] = ch
	h.pendingMu.Unlock()

	msg := &Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  paramsJSON,
	}
	if err := h.send(msg); err != nil {
		h.pendingMu.Lock()
		delete(h.pending, id)
		h.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		h.pendingMu.Lock()
		delete(h.pending, id)
		h.pendingMu.Unlock()
		h.sendCancel(id)
		return nil, ctx.Err()
	case <-h.closed:
		h.pendingMu.Lock()
		delete(h.pending, id)
		h.pendingMu.Unlock()
		return nil, fmt.Errorf("extension %s closed", h.manifest.Name)
	}
}

// sendCancel tells the extension to abort the in-flight request.
func (h *Host) sendCancel(id int) {
	paramsJSON, _ := json.Marshal(CancelParams{ID: id})
	_ = h.send(&Message{
		JSONRPC: "2.0",
		Method:  MethodCancelRequest,
		Params:  paramsJSON,
	})
}

func (h *Host) send(msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	data = append(data, '\n')

	h.writeMu.Lock()
	defer h.writeMu.Unlock()

	_, err = h.stdin.Write(data)
	return err
}

func (h *Host) readLoop() {
	defer func() {
		// Unblock any pending requests when the process exits
		h.closeOnce.Do(func() {
			close(h.closed)
		})
	}()

	for h.stdout.Scan() {
		line := h.stdout.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			slog.Debug("extension bad json", "name", h.manifest.Name, "err", err)
			continue
		}

		h.handleMessage(&msg)
	}
}

func (h *Host) handleMessage(msg *Message) {
	// Response to a pending request (has ID, no method)
	if msg.ID != nil && msg.Method == "" {
		h.pendingMu.Lock()
		ch, ok := h.pending[*msg.ID]
		if ok {
			delete(h.pending, *msg.ID)
		}
		h.pendingMu.Unlock()
		if ok {
			ch <- msg
		}
		return
	}

	// Request from extension to host (has ID + method) — needs a response
	if msg.ID != nil && msg.Method != "" {
		h.handleRequest(msg)
		return
	}

	// Notification from extension (no ID)
	switch msg.Method {
	case MethodRegisterTool:
		var p RegisterToolParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.tools = append(h.tools, p)
		}
	case MethodRegisterCommand:
		var p RegisterCommandParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.commands = append(h.commands, p)
		}
	case MethodRegisterPromptSection:
		var p RegisterPromptSectionParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.promptSections = append(h.promptSections, p)
		}
	case MethodRegisterInterceptor:
		var p RegisterInterceptorParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.interceptors = append(h.interceptors, p)
		}
	case MethodRegisterEventHandler:
		var p RegisterEventHandlerParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.eventHandlers = append(h.eventHandlers, p)
		}
	case MethodRegisterShortcut:
		var p RegisterShortcutParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.shortcuts = append(h.shortcuts, p)
		}
	case MethodRegisterMessageHook:
		var p RegisterMessageHookParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.messageHooks = append(h.messageHooks, p)
		}
	case MethodRegisterCompactor:
		var p RegisterCompactorParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.compactor = &p
		}
	case MethodNotify:
		var p NotifyParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.Notify(p.Message)
		}
	case MethodShowMessage:
		var p ShowMessageParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.ShowMessage(p.Text)
		}
	case MethodSendMessage:
		var p SendMessageParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.SendMessage(p.Content)
		}
	case MethodLog:
		var p LogParams
		if json.Unmarshal(msg.Params, &p) == nil {
			var level slog.Level
			_ = level.UnmarshalText([]byte(p.Level))
			slog.Log(context.Background(), level, p.Message, "ext", h.manifest.Name)
		}
	}
}

// handleRequest processes a request from the extension that expects a response.
func (h *Host) handleRequest(msg *Message) {
	switch msg.Method {
	case MethodHostListTools:
		h.handleHostListTools(msg)
	case MethodHostExecuteTool:
		h.handleHostExecuteTool(msg)
	case MethodHostConfigGet:
		h.handleHostConfigGet(msg)
	case MethodHostConfigReadExt:
		h.handleHostConfigReadExt(msg)
	case MethodHostAuthGetKey:
		h.handleHostAuthGetKey(msg)
	case MethodHostChat:
		go h.handleHostChat(msg) // may be slow (LLM call)
	case MethodHostAgentRun:
		go h.handleHostAgentRun(msg) // may be slow (agent loop)
	case MethodHostConversationMessages:
		h.handleHostConversationMessages(msg)
	case MethodHostSessions:
		h.handleHostSessions(msg)
	case MethodHostLoadSession:
		h.handleHostLoadSession(msg)
	case MethodHostForkSession:
		h.handleHostForkSession(msg)
	case MethodHostSetSessionTitle:
		h.handleHostSetSessionTitle(msg)
	case MethodHostSyncModels:
		h.handleHostSyncModels(msg)
	case MethodHostRunBackground:
		h.handleHostRunBackground(msg)
	case MethodHostCancelBackground:
		h.handleHostCancelBackground(msg)
	case MethodHostIsBackgroundRunning:
		h.handleHostIsBackgroundRunning(msg)
	case MethodHostExtInfos:
		h.handleHostExtInfos(msg)
	case MethodHostExtensionsDir:
		h.handleHostExtensionsDir(msg)
	case MethodHostUndoSnapshots:
		h.handleHostUndoSnapshots(msg)
	case MethodHostUndoRestore:
		h.handleHostUndoRestore(msg)
	default:
		h.respondError(*msg.ID, -32601, "method not found: "+msg.Method)
	}
}

// handleHostListTools returns the list of available host tools with their schemas.
func (h *Host) handleHostListTools(msg *Message) {
	var params HostListToolsParams
	_ = json.Unmarshal(msg.Params, &params)

	if h.app == nil {
		h.respond(*msg.ID, HostListToolsResult{})
		return
	}

	var defs []*ext.ToolDef
	if params.Filter == FilterBackgroundSafe {
		for _, td := range h.app.ToolDefs() {
			if td.BackgroundSafe {
				defs = append(defs, td)
			}
		}
	} else {
		defs = h.app.ToolDefs()
	}

	infos := make([]HostToolInfo, len(defs))
	for i, td := range defs {
		infos[i] = HostToolInfo{
			Name:        td.Name,
			Description: td.Description,
			Parameters:  td.Parameters,
		}
	}
	h.respond(*msg.ID, HostListToolsResult{Tools: infos})
}

// handleHostExecuteTool executes a host-registered tool on behalf of the extension.
func (h *Host) handleHostExecuteTool(msg *Message) {
	var params HostExecuteToolParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}

	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}

	// Look up the tool in the host registry
	tool := h.app.FindTool(params.Name)
	if tool == nil {
		h.respondError(*msg.ID, -32604, "unknown tool: "+params.Name)
		return
	}

	// Execute the tool
	result, err := tool.Execute(context.Background(), "", params.Args)
	if err != nil {
		h.respond(*msg.ID, HostExecuteToolResult{
			Content: []ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
		return
	}

	h.respond(*msg.ID, HostExecuteToolResult{Content: coreToWire(result.Content)})
}

func (h *Host) handleHostConfigGet(msg *Message) {
	var params HostConfigGetParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}

	settings, err := config.Load()
	if err != nil {
		h.respondError(*msg.ID, -32603, "load config: "+err.Error())
		return
	}

	values := make(map[string]any, len(params.Keys))
	for _, key := range params.Keys {
		switch key {
		case "defaultModel":
			values[key] = settings.ResolveDefaultModel()
		case "smallModel":
			values[key] = settings.ResolveSmallModel()
		case "agent.compactAt":
			values[key] = config.IntOr(settings.Agent.CompactAt, 0)
		case "agent.maxTurns":
			values[key] = config.IntOr(settings.Agent.MaxTurns, 10)
		}
	}
	h.respond(*msg.ID, HostConfigGetResult{Values: values})
}

func (h *Host) handleHostConfigReadExt(msg *Message) {
	var params HostConfigReadExtParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}

	content, _ := config.ReadExtensionConfig(params.Name)
	h.respond(*msg.ID, HostConfigReadExtResult{Content: content})
}

func (h *Host) handleHostAuthGetKey(msg *Message) {
	var params HostAuthGetKeyParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}

	auth, err := config.NewAuthDefault()
	if err != nil {
		h.respondError(*msg.ID, -32603, "load auth: "+err.Error())
		return
	}

	key := auth.GetAPIKey(params.Provider)
	h.respond(*msg.ID, HostAuthGetKeyResult{Key: key})
}

func (h *Host) handleHostChat(msg *Message) {
	var params HostChatParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}

	prov, err := h.resolveProvider(params.Model)
	if err != nil {
		h.respondError(*msg.ID, -32603, "resolve provider: "+err.Error())
		return
	}

	msgs := make([]core.Message, len(params.Messages))
	for i, m := range params.Messages {
		switch m.Role {
		case "assistant":
			msgs[i] = &core.AssistantMessage{
				Content: []core.AssistantContent{core.TextContent{Text: m.Content}},
			}
		default:
			msgs[i] = &core.UserMessage{Content: m.Content}
		}
	}

	req := core.StreamRequest{
		System:   params.System,
		Messages: msgs,
	}
	if params.MaxTokens > 0 {
		req.Options.MaxTokens = &params.MaxTokens
	}

	ch := prov.Stream(context.Background(), req)

	var text strings.Builder
	var usage HostTokenUsage
	for evt := range ch {
		switch evt.Type {
		case core.StreamTextDelta:
			text.WriteString(evt.Delta)
		case core.StreamDone:
			if evt.Message != nil {
				usage.Input += evt.Message.Usage.InputTokens
				usage.Output += evt.Message.Usage.OutputTokens
			}
		case core.StreamError:
			h.respondError(*msg.ID, -32603, "chat error: "+evt.Error.Error())
			return
		}
	}

	h.respond(*msg.ID, HostChatResult{Text: text.String(), Usage: usage})
}

func (h *Host) handleHostAgentRun(msg *Message) {
	var params HostAgentRunParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}

	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}

	prov, err := h.resolveProvider(params.Model)
	if err != nil {
		h.respondError(*msg.ID, -32603, "resolve provider: "+err.Error())
		return
	}

	var tools []core.Tool
	if params.Tools == "all" {
		tools = h.app.CoreTools()
	} else {
		tools = h.app.BackgroundSafeTools()
	}

	maxTurns := params.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 10
	}

	sub := core.NewAgent(core.AgentConfig{
		System:   params.System,
		Provider: prov,
		Tools:    tools,
		MaxTurns: maxTurns,
	})

	ch := sub.Start(context.Background(), params.Task)

	var result string
	var totalIn, totalOut, turns int
	for evt := range ch {
		if te, ok := evt.(core.EventTurnEnd); ok {
			turns++
			if te.Assistant != nil {
				totalIn += te.Assistant.Usage.InputTokens
				totalOut += te.Assistant.Usage.OutputTokens
				for _, c := range te.Assistant.Content {
					if tc, ok := c.(core.TextContent); ok {
						result = tc.Text
					}
				}
			}
		}
	}

	h.respond(*msg.ID, HostAgentRunResult{
		Text:  result,
		Turns: turns,
		Usage: HostTokenUsage{Input: totalIn, Output: totalOut},
	})
}

func (h *Host) handleHostConversationMessages(msg *Message) {
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}
	msgs := h.app.ConversationMessages()
	data, err := json.Marshal(msgs)
	if err != nil {
		h.respondError(*msg.ID, -32603, "marshal messages: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostConversationMessagesResult{Messages: data})
}

func (h *Host) handleHostSessions(msg *Message) {
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}
	summaries, err := h.app.Sessions()
	if err != nil {
		h.respondError(*msg.ID, -32603, "sessions: "+err.Error())
		return
	}
	infos := make([]WireSessionInfo, len(summaries))
	for i, s := range summaries {
		infos[i] = WireSessionInfo{
			ID:        s.ID,
			Title:     s.Title,
			CWD:       s.CWD,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			ParentID:  s.ParentID,
			Path:      s.Path,
		}
	}
	h.respond(*msg.ID, HostSessionsResult{Sessions: infos})
}

func (h *Host) handleHostLoadSession(msg *Message) {
	var params HostLoadSessionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}
	if err := h.app.LoadSession(params.Path); err != nil {
		h.respondError(*msg.ID, -32603, "load session: "+err.Error())
		return
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostForkSession(msg *Message) {
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}
	parentID, count, err := h.app.ForkSession()
	if err != nil {
		h.respondError(*msg.ID, -32603, "fork session: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostForkSessionResult{ParentID: parentID, MessageCount: count})
}

func (h *Host) handleHostSetSessionTitle(msg *Message) {
	var params HostSetSessionTitleParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}
	if err := h.app.SetSessionTitle(params.Title); err != nil {
		h.respondError(*msg.ID, -32603, "set session title: "+err.Error())
		return
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostSyncModels(msg *Message) {
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}
	updated, err := h.app.SyncModels()
	if err != nil {
		h.respondError(*msg.ID, -32603, "sync models: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostSyncModelsResult{Updated: updated})
}

func (h *Host) handleHostRunBackground(msg *Message) {
	var params HostRunBackgroundParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return
	}
	if err := h.app.RunBackground(params.Prompt); err != nil {
		h.respondError(*msg.ID, -32603, "run background: "+err.Error())
		return
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostCancelBackground(msg *Message) {
	if h.app != nil {
		h.app.CancelBackground()
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostIsBackgroundRunning(msg *Message) {
	running := h.app != nil && h.app.IsBackgroundRunning()
	h.respond(*msg.ID, HostIsBackgroundRunningResult{Running: running})
}

func (h *Host) handleHostExtInfos(msg *Message) {
	if h.app == nil {
		h.respond(*msg.ID, HostExtInfosResult{})
		return
	}
	infos := h.app.ExtInfos()
	wires := make([]WireExtInfo, len(infos))
	for i, info := range infos {
		wires[i] = WireExtInfo{
			Name:          info.Name,
			Version:       info.Version,
			Kind:          info.Kind,
			Runtime:       info.Runtime,
			Tools:         info.Tools,
			Commands:      info.Commands,
			Interceptors:  info.Interceptors,
			EventHandlers: info.EventHandlers,
			Shortcuts:     info.Shortcuts,
			MessageHooks:  info.MessageHooks,
			Compactor:     info.Compactor,
		}
	}
	h.respond(*msg.ID, HostExtInfosResult{Extensions: wires})
}

func (h *Host) handleHostExtensionsDir(msg *Message) {
	dir, err := ExtensionsDir()
	if err != nil {
		h.respondError(*msg.ID, -32603, "extensions dir: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostExtensionsDirResult{Path: dir})
}

func (h *Host) handleHostUndoSnapshots(msg *Message) {
	snapshots, err := tool.UndoSnapshots()
	if err != nil {
		h.respondError(*msg.ID, -32603, "undo snapshots: "+err.Error())
		return
	}
	sizes := make(map[string]int, len(snapshots))
	for path, data := range snapshots {
		sizes[path] = len(data)
	}
	h.respond(*msg.ID, HostUndoSnapshotsResult{Snapshots: sizes})
}

func (h *Host) handleHostUndoRestore(msg *Message) {
	var params HostUndoRestoreParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return
	}
	snapshots, err := tool.UndoSnapshots()
	if err != nil {
		h.respondError(*msg.ID, -32603, "undo snapshots: "+err.Error())
		return
	}
	data, ok := snapshots[params.Path]
	if !ok {
		h.respondError(*msg.ID, -32604, "no snapshot for path: "+params.Path)
		return
	}
	if err := os.WriteFile(params.Path, data, 0600); err != nil {
		h.respondError(*msg.ID, -32603, "write file: "+err.Error())
		return
	}
	h.respond(*msg.ID, struct{}{})
}

// resolveProvider creates a StreamProvider for the given model specifier.
// Model can be "small", "default", or an explicit model ID.
func (h *Host) resolveProvider(model string) (core.StreamProvider, error) {
	auth, err := config.NewAuthDefault()
	if err != nil {
		return nil, fmt.Errorf("load auth: %w", err)
	}

	settings, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	modelQuery := model
	switch model {
	case "", "default":
		modelQuery = settings.ResolveDefaultModel()
	case "small":
		modelQuery = settings.ResolveSmallModel()
	}
	if modelQuery == "" {
		return nil, fmt.Errorf("no model configured")
	}

	registry := provider.NewRegistry()
	resolved, ok := registry.Resolve(modelQuery)
	if !ok {
		return nil, fmt.Errorf("unknown model: %s", modelQuery)
	}

	return registry.Create(resolved, func() string {
		return auth.GetAPIKey(resolved.Provider)
	})
}

func (h *Host) respond(id int, result any) {
	data, _ := json.Marshal(result)
	_ = h.send(&Message{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  data,
	})
}

func (h *Host) respondError(id int, code int, message string) {
	_ = h.send(&Message{
		JSONRPC: "2.0",
		ID:      &id,
		Error:   &RPCError{Code: code, Message: message},
	})
}

// logWriter forwards extension stderr to slog.
type logWriter struct {
	name string
}

func (w *logWriter) Write(p []byte) (int, error) {
	slog.Debug(strings.TrimRight(string(p), "\n"), "ext_stderr", w.name)
	return len(p), nil
}
