package external

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

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
		slog.Debug("readLoop: exiting", "ext", h.manifest.Name)
		// Signal readLoop exit BEFORE closeOnce — Stop() waits on readDone
		// inside its own closeOnce.Do, so reversing this order deadlocks.
		close(h.readDone)
		// Crash path: if Stop() was never called, close h.closed to
		// unblock any pending requests.
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

	if err := h.stdout.Err(); err != nil {
		slog.Debug("readLoop: scanner error", "ext", h.manifest.Name, "err", err)
	} else {
		slog.Debug("readLoop: scanner EOF", "ext", h.manifest.Name)
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
	case MethodRegisterInputTransformer:
		var p RegisterInputTransformerParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.inputTransformers = append(h.inputTransformers, p)
		}
	case MethodRegisterProvider:
		var p RegisterProviderParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.providers = append(h.providers, p)
		}
	case MethodSetWidget:
		var p SetWidgetParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.SetWidget(p.Key, p.Placement, p.Content)
		}
	case MethodShowOverlay:
		var p ShowOverlayParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.ShowOverlay(p.Key, p.Title, p.Content, p.Anchor, p.Width)
		}
	case MethodCloseOverlay:
		var p CloseOverlayParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.CloseOverlay(p.Key)
		}
	case MethodNotify:
		var p NotifyParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.NotifyWithLevel(p.Message, p.Level)
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
	case MethodSteer:
		var p SteerParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.Steer(p.Content)
		}
	case MethodLog:
		var p LogParams
		if json.Unmarshal(msg.Params, &p) == nil {
			var level slog.Level
			_ = level.UnmarshalText([]byte(p.Level))
			slog.Log(context.Background(), level, p.Message, "ext", h.manifest.Name)
		}
	case MethodProviderDelta:
		var p ProviderDeltaParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.deltaMu.Lock()
			ch, ok := h.deltaChans[p.RequestID]
			h.deltaMu.Unlock()
			if ok {
				select {
				case ch <- p:
				default:
					slog.Debug("provider delta dropped (channel full)", "ext", h.manifest.Name, "requestID", p.RequestID)
				}
			}
		}
	}
}

// handleRequest processes a request from the extension that expects a response.
func (h *Host) handleRequest(msg *Message) {
	switch msg.Method {
	case MethodHostListTools:
		h.handleHostListTools(msg)
	case MethodHostExecuteTool:
		go h.handleHostExecuteTool(msg) // may be slow (tool execution)
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
	case MethodHostSetConversationMessages:
		h.handleHostSetConversationMessages(msg)
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
	case MethodHostLastAssistantText:
		h.handleHostLastAssistantText(msg)
	case MethodHostAppendSessionEntry:
		h.handleHostAppendSessionEntry(msg)
	case MethodHostAppendCustomMessage:
		h.handleHostAppendCustomMessage(msg)
	case MethodHostSetLabel:
		h.handleHostSetLabel(msg)
	case MethodHostBranchSession:
		h.handleHostBranchSession(msg)
	case MethodHostBranchSessionSummary:
		h.handleHostBranchSessionSummary(msg)
	case MethodHostPublish:
		h.handleHostPublish(msg)
	case MethodHostSubscribe:
		h.handleHostSubscribe(msg)
	default:
		h.respondError(*msg.ID, -32601, "method not found: "+msg.Method)
	}
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
