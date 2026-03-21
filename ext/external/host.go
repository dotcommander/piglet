package external

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dotcommander/piglet/ext"
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

	// Registrations collected during handshake
	tools          []RegisterToolParams
	commands       []RegisterCommandParams
	promptSections []RegisterPromptSectionParams
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
	select {
	case <-h.closed:
		return
	default:
	}

	// Best-effort shutdown notification
	_ = h.send(&Message{
		JSONRPC: "2.0",
		Method:  MethodShutdown,
	})

	h.stdin.Close()
	close(h.closed)

	if h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
	_ = h.cmd.Wait()
}

// Name returns the extension name from the manifest.
func (h *Host) Name() string { return h.manifest.Name }

// Tools returns the tools registered during handshake.
func (h *Host) Tools() []RegisterToolParams { return h.tools }

// Commands returns the commands registered during handshake.
func (h *Host) Commands() []RegisterCommandParams { return h.commands }

// PromptSections returns the prompt sections registered during handshake.
func (h *Host) PromptSections() []RegisterPromptSectionParams { return h.promptSections }

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

	defer func() {
		h.pendingMu.Lock()
		delete(h.pending, id)
		h.pendingMu.Unlock()
	}()

	msg := &Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  paramsJSON,
	}
	if err := h.send(msg); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		h.sendCancel(id)
		return nil, ctx.Err()
	case <-h.closed:
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
		select {
		case <-h.closed:
		default:
			close(h.closed)
		}
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
	// Response to a pending request
	if msg.ID != nil && msg.Method == "" {
		h.pendingMu.Lock()
		ch, ok := h.pending[*msg.ID]
		h.pendingMu.Unlock()
		if ok {
			ch <- msg
		}
		return
	}

	// Notification from extension
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
	case MethodLog:
		var p LogParams
		if json.Unmarshal(msg.Params, &p) == nil {
			var level slog.Level
			_ = level.UnmarshalText([]byte(p.Level))
			slog.Log(context.Background(), level, p.Message, "ext", h.manifest.Name)
		}
	}
}

// logWriter forwards extension stderr to slog.
type logWriter struct {
	name string
}

func (w *logWriter) Write(p []byte) (int, error) {
	slog.Debug(strings.TrimRight(string(p), "\n"), "ext_stderr", w.name)
	return len(p), nil
}
