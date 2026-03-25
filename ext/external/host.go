package external

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// providerResolverFn resolves a model specifier to a StreamProvider.
type providerResolverFn func(model string) (core.StreamProvider, error)

// Host manages a single external extension process.
type Host struct {
	manifest           *Manifest
	cwd                string
	app                *ext.App // nil until bridge wires it
	resolveProviderFn  providerResolverFn

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

// SetProviderResolver sets the function used to resolve a model to a StreamProvider.
func (h *Host) SetProviderResolver(fn providerResolverFn) { h.resolveProviderFn = fn }

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
