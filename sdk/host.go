package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Generic RPC helpers
// ---------------------------------------------------------------------------

// hostCall sends a JSON-RPC request and unmarshals the result into T.
func hostCall[T any](e *Extension, ctx context.Context, method string, params any) (T, error) {
	var zero T
	resp, err := e.request(ctx, method, params)
	if err != nil {
		return zero, err
	}
	if resp.Error != nil {
		return zero, fmt.Errorf("%s: %s", method, resp.Error.Message)
	}
	var result T
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return zero, fmt.Errorf("unmarshal %s: %w", method, err)
	}
	return result, nil
}

// hostCallVoid sends a JSON-RPC request that returns no data.
func hostCallVoid(e *Extension, ctx context.Context, method string, params any) error {
	resp, err := e.request(ctx, method, params)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s: %s", method, resp.Error.Message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Host service methods (extension → host)
// ---------------------------------------------------------------------------

// ConfigGet retrieves configuration values from the host.
// Keys use dot notation (e.g. "defaultModel", "agent.compactAt").
// Returns a map of key → value. Missing keys are omitted.
func (e *Extension) ConfigGet(ctx context.Context, keys ...string) (map[string]any, error) {
	type resp struct {
		Values map[string]any `json:"values"`
	}
	r, err := hostCall[resp](e, ctx, "host/config.get", map[string]any{"keys": keys})
	if err != nil {
		return nil, err
	}
	return r.Values, nil
}

// ConfigReadExtension reads an extension's markdown config file from
// ~/.config/piglet/<name>.md via the host.
//
// Deprecated: Extensions should read their own config files directly from
// e.ConfigDir() after initialization. This method is retained for backward
// compatibility.
func (e *Extension) ConfigReadExtension(ctx context.Context, name string) (string, error) {
	type resp struct {
		Content string `json:"content"`
	}
	r, err := hostCall[resp](e, ctx, "host/config.readExtension", map[string]any{"name": name})
	if err != nil {
		return "", err
	}
	return r.Content, nil
}

// AuthGetKey retrieves an API key for a provider from the host's auth store.
func (e *Extension) AuthGetKey(ctx context.Context, provider string) (string, error) {
	type resp struct {
		Key string `json:"key"`
	}
	r, err := hostCall[resp](e, ctx, "host/auth.getKey", map[string]any{"provider": provider})
	if err != nil {
		return "", err
	}
	return r.Key, nil
}

// Chat makes a single-turn LLM call via the host. The host handles model
// resolution, provider creation, and streaming. Use for lightweight calls
// like title generation or summary refinement.
func (e *Extension) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	r, err := hostCall[ChatResponse](e, ctx, "host/chat", req)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RunAgent asks the host to run a full agent loop with tools to completion.
// The host handles model resolution, tool access, and multi-turn execution.
// Returns the final agent text output and usage statistics.
func (e *Extension) RunAgent(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
	r, err := hostCall[AgentResponse](e, ctx, "host/agent.run", req)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListHostTools returns the schemas of tools available in the host.
// Filter can be "all" (default) or "background_safe".
func (e *Extension) ListHostTools(ctx context.Context, filter string) ([]HostToolInfo, error) {
	type resp struct {
		Tools []HostToolInfo `json:"tools"`
	}
	r, err := hostCall[resp](e, ctx, "host/listTools", map[string]any{"filter": filter})
	if err != nil {
		return nil, err
	}
	return r.Tools, nil
}

// CallHostTool executes a host-registered tool and returns the result.
// This allows extensions to use tools like Read, Edit, Grep, Bash that are
// registered in the host process. The call blocks until the host responds.
// callID is optional; when provided, it correlates the tool result with the
// original tool call for event handlers and session persistence.
func (e *Extension) CallHostTool(ctx context.Context, name string, args map[string]any, callID ...string) (*ToolResult, error) {
	params := map[string]any{
		"name": name,
		"args": args,
	}
	if len(callID) > 0 && callID[0] != "" {
		params["callId"] = callID[0]
	}
	resp, err := e.request(ctx, "host/executeTool", params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("host tool %s: %s", name, resp.Error.Message)
	}

	var result struct {
		Content []ContentBlock `json:"content"`
		IsError bool           `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal host tool result: %w", err)
	}
	return &ToolResult{Content: result.Content, IsError: result.IsError}, nil
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

// ConversationMessages returns the current conversation history as raw JSON.
func (e *Extension) ConversationMessages(ctx context.Context) (json.RawMessage, error) {
	type resp struct {
		Messages json.RawMessage `json:"messages"`
	}
	r, err := hostCall[resp](e, ctx, "host/conversationMessages", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Messages, nil
}

// LLMSnapshot returns a read-only projection of what would be sent to the LLM:
// system prompt, conversation messages, and tool schemas.
func (e *Extension) LLMSnapshot(ctx context.Context) (LLMSnapshotResult, error) {
	return hostCall[LLMSnapshotResult](e, ctx, "host/llmSnapshot", struct{}{})
}

// LLMSnapshotResult is the response from host/llmSnapshot.
type LLMSnapshotResult struct {
	System   string          `json:"system"`
	Messages json.RawMessage `json:"messages"`
	Tools    json.RawMessage `json:"tools"`
}

// SetConversationMessages replaces the conversation history with the given wire messages.
func (e *Extension) SetConversationMessages(ctx context.Context, messages json.RawMessage) error {
	return hostCallVoid(e, ctx, "host/setConversationMessages", map[string]any{
		"messages": messages,
	})
}

// Sessions returns all session summaries from the host.
func (e *Extension) Sessions(ctx context.Context) ([]SessionInfo, error) {
	type resp struct {
		Sessions []SessionInfo `json:"sessions"`
	}
	r, err := hostCall[resp](e, ctx, "host/sessions", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Sessions, nil
}

// ForkSession forks the current session and returns the parent ID and message count.
func (e *Extension) ForkSession(ctx context.Context) (parentID string, count int, err error) {
	type resp struct {
		ParentID     string `json:"parentID"`
		MessageCount int    `json:"messageCount"`
	}
	r, err := hostCall[resp](e, ctx, "host/forkSession", struct{}{})
	if err != nil {
		return "", 0, err
	}
	return r.ParentID, r.MessageCount, nil
}

// SetSessionTitle sets the current session's title.
func (e *Extension) SetSessionTitle(ctx context.Context, title string) error {
	return hostCallVoid(e, ctx, "host/setSessionTitle", map[string]any{"title": title})
}

// SyncModels syncs the model catalog and returns the count of updated models.
func (e *Extension) SyncModels(ctx context.Context) (int, error) {
	type resp struct {
		Updated int `json:"updated"`
	}
	r, err := hostCall[resp](e, ctx, "host/syncModels", struct{}{})
	if err != nil {
		return 0, err
	}
	return r.Updated, nil
}

// ModelOverride holds API-sourced values that replace curated defaults.
type ModelOverride struct {
	Name          string `json:"name,omitempty"`
	ContextWindow int    `json:"contextWindow,omitempty"`
	MaxTokens     int    `json:"maxTokens,omitempty"`
}

// WriteModels regenerates models.yaml from the embedded curated list with
// the given API overrides, writes to disk, and reloads the registry.
func (e *Extension) WriteModels(ctx context.Context, overrides map[string]ModelOverride) (int, error) {
	type resp struct {
		ModelsWritten int `json:"modelsWritten"`
	}
	r, err := hostCall[resp](e, ctx, "host/writeModels", map[string]any{"overrides": overrides})
	if err != nil {
		return 0, err
	}
	return r.ModelsWritten, nil
}

// RunBackground starts a background agent with the given prompt.
func (e *Extension) RunBackground(ctx context.Context, prompt string) error {
	return hostCallVoid(e, ctx, "host/runBackground", map[string]any{"prompt": prompt})
}

// CancelBackground cancels the running background agent.
func (e *Extension) CancelBackground(ctx context.Context) error {
	return hostCallVoid(e, ctx, "host/cancelBackground", struct{}{})
}

// IsBackgroundRunning returns whether a background agent is currently active.
func (e *Extension) IsBackgroundRunning(ctx context.Context) (bool, error) {
	type resp struct {
		Running bool `json:"running"`
	}
	r, err := hostCall[resp](e, ctx, "host/isBackgroundRunning", struct{}{})
	if err != nil {
		return false, err
	}
	return r.Running, nil
}

// ExtInfos returns metadata about all loaded extensions.
func (e *Extension) ExtInfos(ctx context.Context) ([]ExtInfo, error) {
	type resp struct {
		Extensions []ExtInfo `json:"extensions"`
	}
	r, err := hostCall[resp](e, ctx, "host/extInfos", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Extensions, nil
}

// ExtensionsDir returns the path to the extensions directory.
func (e *Extension) ExtensionsDir(ctx context.Context) (string, error) {
	type resp struct {
		Path string `json:"path"`
	}
	r, err := hostCall[resp](e, ctx, "host/extensionsDir", struct{}{})
	if err != nil {
		return "", err
	}
	return r.Path, nil
}

// UndoSnapshots returns a map of file path to snapshot size in bytes.
func (e *Extension) UndoSnapshots(ctx context.Context) (map[string]int, error) {
	type resp struct {
		Snapshots map[string]int `json:"snapshots"`
	}
	r, err := hostCall[resp](e, ctx, "host/undoSnapshots", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Snapshots, nil
}

// UndoRestore restores a file from its undo snapshot.
func (e *Extension) UndoRestore(ctx context.Context, path string) error {
	return hostCallVoid(e, ctx, "host/undoRestore", map[string]any{"path": path})
}

// LastAssistantText returns the text content of the last assistant message.
// Returns empty string if no assistant messages exist.
func (e *Extension) LastAssistantText(ctx context.Context) (string, error) {
	type resp struct {
		Text string `json:"text"`
	}
	r, err := hostCall[resp](e, ctx, "host/lastAssistantText", struct{}{})
	if err != nil {
		return "", err
	}
	return r.Text, nil
}

// AppendSessionEntry writes a custom extension entry to the current session.
// The kind should be namespaced (e.g., "ext:memory:facts").
func (e *Extension) AppendSessionEntry(ctx context.Context, kind string, data any) error {
	return hostCallVoid(e, ctx, "host/appendSessionEntry", map[string]any{
		"kind": kind,
		"data": data,
	})
}

// AppendCustomMessage writes a message that persists AND appears in Messages() on reload.
// Role must be "user" or "assistant". Use for durable context annotations.
func (e *Extension) AppendCustomMessage(ctx context.Context, role, content string) error {
	return hostCallVoid(e, ctx, "host/appendCustomMessage", map[string]any{
		"role":    role,
		"content": content,
	})
}

// SetLabel sets or clears a bookmark label on a session entry.
// An empty label clears the bookmark.
func (e *Extension) SetLabel(ctx context.Context, targetID, label string) error {
	return hostCallVoid(e, ctx, "host/setLabel", map[string]any{
		"targetId": targetID,
		"label":    label,
	})
}

// BranchSession moves the current session's leaf to an earlier entry.
func (e *Extension) BranchSession(ctx context.Context, entryID string) error {
	return hostCallVoid(e, ctx, "host/branchSession", map[string]any{"entryId": entryID})
}

// BranchSessionWithSummary moves the leaf and writes a branch_summary entry.
func (e *Extension) BranchSessionWithSummary(ctx context.Context, entryID, summary string) error {
	return hostCallVoid(e, ctx, "host/branchSessionWithSummary", map[string]any{
		"entryId": entryID,
		"summary": summary,
	})
}

// Publish sends data to all subscribers of an event bus topic.
func (e *Extension) Publish(ctx context.Context, topic string, data any) error {
	return hostCallVoid(e, ctx, "host/publish", map[string]any{
		"topic": topic,
		"data":  data,
	})
}

// Subscription tracks an event bus subscription for cleanup.
type Subscription struct {
	ID    int
	topic string
	ext   *Extension
	ch    chan json.RawMessage
}

// Events returns a channel that receives events for this subscription.
func (s *Subscription) Events() <-chan json.RawMessage { return s.ch }

// Subscribe registers for events on a topic. Returns a Subscription whose
// Events() channel receives data whenever the topic fires. Call Unsubscribe()
// when done. Events are delivered as json.RawMessage — unmarshal to your type.
func (e *Extension) Subscribe(ctx context.Context, topic string) (*Subscription, error) {
	type resp struct {
		SubscriptionID int `json:"subscriptionId"`
	}
	r, err := hostCall[resp](e, ctx, "host/subscribe", map[string]any{"topic": topic})
	if err != nil {
		return nil, err
	}

	sub := &Subscription{
		ID:    r.SubscriptionID,
		topic: topic,
		ext:   e,
		ch:    make(chan json.RawMessage, 64),
	}

	e.subsMu.Lock()
	e.subs[sub.ID] = sub
	e.subsMu.Unlock()

	return sub, nil
}

// Unsubscribe removes the subscription. The Events() channel is closed.
func (s *Subscription) Unsubscribe() {
	s.ext.subsMu.Lock()
	delete(s.ext.subs, s.ID)
	s.ext.subsMu.Unlock()
	close(s.ch)
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
		// Notify host to cancel work for this request
		e.sendNotification("$/cancelRequest", map[string]int{"id": id})
		// Drain any late-arriving response (50ms grace)
		t := time.NewTimer(50 * time.Millisecond)
		select {
		case resp := <-ch:
			t.Stop()
			return resp, nil
		case <-t.C:
			return nil, ctx.Err()
		}
	}
}
