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
	undoSnapshotsFn    func() (map[string][]byte, error)

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	rpcRead io.Closer // ext→host read end (for cleanup)
	stdout  *bufio.Scanner

	ctx    context.Context
	cancel context.CancelFunc

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
	providers      []RegisterProviderParams // collected during handshake
	deltaMu        sync.Mutex
	deltaChans     map[int]chan ProviderDeltaParams // requestId → delta channel
}

// NewHost creates a host for the given manifest.
func NewHost(m *Manifest, cwd string) *Host {
	return &Host{
		manifest:   m,
		cwd:        cwd,
		pending:    make(map[int]chan *Message),
		closed:     make(chan struct{}),
		deltaChans: make(map[int]chan ProviderDeltaParams),
	}
}

// SetApp wires the host to the main application for runtime notifications.
func (h *Host) SetApp(app *ext.App) { h.app = app }

// SetProviderResolver sets the function used to resolve a model to a StreamProvider.
func (h *Host) SetProviderResolver(fn providerResolverFn) { h.resolveProviderFn = fn }

// SetUndoSnapshots sets the function used to retrieve undo snapshots.
func (h *Host) SetUndoSnapshots(fn func() (map[string][]byte, error)) { h.undoSnapshotsFn = fn }

// Start spawns the extension process, performs the initialize handshake,
// and collects registrations. Returns after the extension sends initialize result.
func (h *Host) Start(ctx context.Context) error {
	h.ctx, h.cancel = context.WithCancel(ctx)

	bin, args := h.manifest.RuntimeCommand()
	h.cmd = exec.CommandContext(ctx, bin, args...)
	h.cmd.Dir = h.manifest.Dir

	// Create anonymous pipe pairs for JSON-RPC (FD 3/4 in child)
	// Pair 1: host→ext (host writes, child reads on FD 3)
	extRead, hostWrite, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create host→ext pipe: %w", err)
	}
	// Pair 2: ext→host (child writes on FD 4, host reads)
	hostRead, extWrite, err := os.Pipe()
	if err != nil {
		extRead.Close()
		hostWrite.Close()
		return fmt.Errorf("create ext→host pipe: %w", err)
	}

	h.cmd.ExtraFiles = []*os.File{extRead, extWrite} // become FD 3, FD 4
	h.cmd.Stdin = nil                                  // extensions don't read stdin
	h.cmd.Stdout = &logWriter{name: h.manifest.Name + "/stdout"} // capture stray prints
	h.cmd.Stderr = &logWriter{name: h.manifest.Name}

	h.stdin = hostWrite
	h.rpcRead = hostRead
	h.stdout = bufio.NewScanner(hostRead)
	h.stdout.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	if err := h.cmd.Start(); err != nil {
		hostWrite.Close()
		hostRead.Close()
		extRead.Close()
		extWrite.Close()
		return fmt.Errorf("start %s: %w", h.manifest.Name, err)
	}

	// Close child-side pipe ends after fork
	extRead.Close()
	extWrite.Close()

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
		if h.cancel != nil {
			h.cancel()
		}

		// Best-effort shutdown notification
		_ = h.send(&Message{
			JSONRPC: "2.0",
			Method:  MethodShutdown,
		})

		_ = h.stdin.Close()
		if h.rpcRead != nil {
			_ = h.rpcRead.Close()
		}
		close(h.closed)

		if h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
		_ = h.cmd.Wait()
	})
}

// Name returns the extension name from the manifest.
func (h *Host) Name() string { return h.manifest.Name }

// Closed returns a channel that is closed when the host process exits.
func (h *Host) Closed() <-chan struct{} { return h.closed }

// releaseDeltaChan removes a delta channel from the map, stopping further sends.
func (h *Host) releaseDeltaChan(id int) {
	h.deltaMu.Lock()
	delete(h.deltaChans, id)
	h.deltaMu.Unlock()
}

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

// Providers returns the providers registered during handshake.
func (h *Host) Providers() []RegisterProviderParams { return h.providers }

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
