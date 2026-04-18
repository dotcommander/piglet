package sdk

import (
	"context"
	"encoding/json"
	"fmt"
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

// ActivateHostTool promotes a deferred tool to full schema.
func (e *Extension) ActivateHostTool(ctx context.Context, name string) error {
	return hostCallVoid(e, ctx, "host/activateTool", map[string]any{"name": name})
}

// SetToolFilter sets a per-turn tool filter. Only the named tools will be
// included in the agent's tool set. Empty/nil names clears the filter.
func (e *Extension) SetToolFilter(ctx context.Context, names []string) error {
	return hostCallVoid(e, ctx, "host/setToolFilter", map[string]any{"names": names})
}
