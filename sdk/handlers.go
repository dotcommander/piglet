package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
)

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

// unmarshalParams unmarshals msg.Params into v, returning false on error.
func (e *Extension) unmarshalParams(msg *rpcMessage, v any) bool {
	if err := json.Unmarshal(msg.Params, v); err != nil {
		e.sendError(*msg.ID, -32600, fmt.Sprintf("invalid params: %v", err))
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Request handlers
// ---------------------------------------------------------------------------

func (e *Extension) handleInitialize(msg *rpcMessage) {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
		CWD             string `json:"cwd"`
		ConfigDir       string `json:"configDir"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}
	e.cwd = params.CWD
	e.configDir = params.ConfigDir

	// Call OnInit hook (allows lazy registration that needs CWD)
	if e.onInit != nil {
		e.onInit(e)
	}

	e.sendRegistrations()

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
	if !e.unmarshalParams(msg, &params) {
		return
	}

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

	resp := map[string]any{
		"content": result.Content,
		"isError": result.IsError,
	}
	if result.ErrorCode != "" {
		resp["errorCode"] = result.ErrorCode
	}
	if result.ErrorHint != "" {
		resp["errorHint"] = result.ErrorHint
	}
	e.sendResponse(*msg.ID, resp)
}

func (e *Extension) handleCommandExecute(msg *rpcMessage) {
	var params struct {
		Name string `json:"name"`
		Args string `json:"args"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

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
		Name     string         `json:"name"`
		ToolName string         `json:"toolName"`
		Args     map[string]any `json:"args"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	// Run the targeted interceptor's Before hook (or all if name is empty).
	allow := true
	args := maps.Clone(params.Args)
	var preview string
	for _, ic := range e.interceptors {
		if ic.Before == nil {
			continue
		}
		if params.Name != "" && ic.Name != params.Name {
			continue
		}
		a, modified, err := ic.Before(ctx, params.ToolName, args)
		if err != nil {
			e.sendError(*msg.ID, -32603, err.Error())
			return
		}
		if !a {
			allow = false
			if ic.Preview != nil {
				preview = ic.Preview(ctx, params.ToolName, args)
			}
			break
		}
		if modified != nil {
			args = maps.Clone(modified)
		}
	}

	resp := map[string]any{
		"allow": allow,
		"args":  args,
	}
	if preview != "" {
		resp["preview"] = preview
	}
	e.sendResponse(*msg.ID, resp)
}

func (e *Extension) handleInterceptorAfter(msg *rpcMessage) {
	var params struct {
		Name     string `json:"name"`
		ToolName string `json:"toolName"`
		Details  any    `json:"details"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	details := params.Details
	for _, ic := range e.interceptors {
		if ic.After == nil {
			continue
		}
		if params.Name != "" && ic.Name != params.Name {
			continue
		}
		modified, err := ic.After(ctx, params.ToolName, details)
		if err != nil {
			e.Log("error", fmt.Sprintf("interceptor %q after hook: %v", ic.Name, err))
			continue
		}
		details = modified
	}

	e.sendResponse(*msg.ID, map[string]any{"details": details})
}
