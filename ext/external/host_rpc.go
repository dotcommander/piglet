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
	h.handleNotification(msg)
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
	case MethodHostLLMSnapshot:
		h.handleHostLLMSnapshot(msg)
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
	case MethodHostWriteModels:
		h.handleHostWriteModels(msg)
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
	case MethodHostWaitForIdle:
		go h.handleHostWaitForIdle(msg) // may block until agent is idle
	case MethodHostSetLabel:
		h.handleHostSetLabel(msg)
	case MethodHostBranchSession:
		h.handleHostBranchSession(msg)
	case MethodHostBranchSessionSummary:
		h.handleHostBranchSessionSummary(msg)
	case MethodHostResetSessionLeaf:
		h.handleHostResetSessionLeaf(msg)
	case MethodHostSessionEntryInfos:
		h.handleHostSessionEntryInfos(msg)
	case MethodHostSessionFullTree:
		h.handleHostSessionFullTree(msg)
	case MethodHostSessionTitle:
		h.handleHostSessionTitle(msg)
	case MethodHostSessionStats:
		h.handleHostSessionStats(msg)
	case MethodHostShowPicker:
		h.handleHostShowPicker(msg)
	case MethodHostAvailableModels:
		h.handleHostAvailableModels(msg)
	case MethodHostSwitchModel:
		h.handleHostSwitchModel(msg)
	case MethodHostPublish:
		h.handleHostPublish(msg)
	case MethodHostSubscribe:
		h.handleHostSubscribe(msg)
	case MethodHostActivateTool:
		h.handleHostActivateTool(msg)
	case MethodHostSetToolFilter:
		h.handleHostSetToolFilter(msg)
	case MethodHostToggleStepMode:
		h.handleHostToggleStepMode(msg)
	case MethodHostRequestQuit:
		h.handleHostRequestQuit(msg)
	case MethodHostHasCompactor:
		h.handleHostHasCompactor(msg)
	case MethodHostTriggerCompact:
		go h.handleHostTriggerCompact(msg) // may be slow (LLM compaction)
	case MethodHostCommands:
		h.handleHostCommands(msg)
	case MethodHostToolDefs:
		h.handleHostToolDefs(msg)
	case MethodHostShortcuts:
		h.handleHostShortcuts(msg)
	case MethodHostPromptSections:
		h.handleHostPromptSections(msg)
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
