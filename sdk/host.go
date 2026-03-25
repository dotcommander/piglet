package sdk

import (
	"context"
	"encoding/json"
	"fmt"
)

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

// ConversationMessages returns the current conversation history as raw JSON.
func (e *Extension) ConversationMessages(ctx context.Context) (json.RawMessage, error) {
	resp, err := e.request(ctx, "host/conversationMessages", struct{}{})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("conversation messages: %s", resp.Error.Message)
	}
	var result struct {
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal conversation messages: %w", err)
	}
	return result.Messages, nil
}

// Sessions returns all session summaries from the host.
func (e *Extension) Sessions(ctx context.Context) ([]SessionInfo, error) {
	resp, err := e.request(ctx, "host/sessions", struct{}{})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("sessions: %s", resp.Error.Message)
	}
	var result struct {
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal sessions: %w", err)
	}
	return result.Sessions, nil
}

// LoadSession loads a session by path.
func (e *Extension) LoadSession(ctx context.Context, path string) error {
	resp, err := e.request(ctx, "host/loadSession", map[string]any{"path": path})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("load session: %s", resp.Error.Message)
	}
	return nil
}

// ForkSession forks the current session and returns the parent ID and message count.
func (e *Extension) ForkSession(ctx context.Context) (parentID string, count int, err error) {
	resp, err := e.request(ctx, "host/forkSession", struct{}{})
	if err != nil {
		return "", 0, err
	}
	if resp.Error != nil {
		return "", 0, fmt.Errorf("fork session: %s", resp.Error.Message)
	}
	var result struct {
		ParentID     string `json:"parentID"`
		MessageCount int    `json:"messageCount"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", 0, fmt.Errorf("unmarshal fork session: %w", err)
	}
	return result.ParentID, result.MessageCount, nil
}

// SetSessionTitle sets the current session's title.
func (e *Extension) SetSessionTitle(ctx context.Context, title string) error {
	resp, err := e.request(ctx, "host/setSessionTitle", map[string]any{"title": title})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("set session title: %s", resp.Error.Message)
	}
	return nil
}

// SyncModels syncs the model catalog and returns the count of updated models.
func (e *Extension) SyncModels(ctx context.Context) (int, error) {
	resp, err := e.request(ctx, "host/syncModels", struct{}{})
	if err != nil {
		return 0, err
	}
	if resp.Error != nil {
		return 0, fmt.Errorf("sync models: %s", resp.Error.Message)
	}
	var result struct {
		Updated int `json:"updated"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return 0, fmt.Errorf("unmarshal sync models: %w", err)
	}
	return result.Updated, nil
}

// RunBackground starts a background agent with the given prompt.
func (e *Extension) RunBackground(ctx context.Context, prompt string) error {
	resp, err := e.request(ctx, "host/runBackground", map[string]any{"prompt": prompt})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("run background: %s", resp.Error.Message)
	}
	return nil
}

// CancelBackground cancels the running background agent.
func (e *Extension) CancelBackground(ctx context.Context) error {
	resp, err := e.request(ctx, "host/cancelBackground", struct{}{})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("cancel background: %s", resp.Error.Message)
	}
	return nil
}

// IsBackgroundRunning returns whether a background agent is currently active.
func (e *Extension) IsBackgroundRunning(ctx context.Context) (bool, error) {
	resp, err := e.request(ctx, "host/isBackgroundRunning", struct{}{})
	if err != nil {
		return false, err
	}
	if resp.Error != nil {
		return false, fmt.Errorf("is background running: %s", resp.Error.Message)
	}
	var result struct {
		Running bool `json:"running"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return false, fmt.Errorf("unmarshal is background running: %w", err)
	}
	return result.Running, nil
}

// ExtInfos returns metadata about all loaded extensions.
func (e *Extension) ExtInfos(ctx context.Context) ([]ExtInfo, error) {
	resp, err := e.request(ctx, "host/extInfos", struct{}{})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("ext infos: %s", resp.Error.Message)
	}
	var result struct {
		Extensions []ExtInfo `json:"extensions"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal ext infos: %w", err)
	}
	return result.Extensions, nil
}

// ExtensionsDir returns the path to the extensions directory.
func (e *Extension) ExtensionsDir(ctx context.Context) (string, error) {
	resp, err := e.request(ctx, "host/extensionsDir", struct{}{})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("extensions dir: %s", resp.Error.Message)
	}
	var result struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("unmarshal extensions dir: %w", err)
	}
	return result.Path, nil
}

// UndoSnapshots returns a map of file path to snapshot size in bytes.
func (e *Extension) UndoSnapshots(ctx context.Context) (map[string]int, error) {
	resp, err := e.request(ctx, "host/undoSnapshots", struct{}{})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("undo snapshots: %s", resp.Error.Message)
	}
	var result struct {
		Snapshots map[string]int `json:"snapshots"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal undo snapshots: %w", err)
	}
	return result.Snapshots, nil
}

// UndoRestore restores a file from its undo snapshot.
func (e *Extension) UndoRestore(ctx context.Context, path string) error {
	resp, err := e.request(ctx, "host/undoRestore", map[string]any{"path": path})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("undo restore: %s", resp.Error.Message)
	}
	return nil
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
