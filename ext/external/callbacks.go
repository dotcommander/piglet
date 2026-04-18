package external

import (
	"context"
	"encoding/json"
	"fmt"
)

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
func (h *Host) InterceptBefore(ctx context.Context, name, toolName string, args map[string]any) (bool, map[string]any, string, error) {
	resp, err := h.request(ctx, MethodInterceptorBefore, InterceptorBeforeParams{
		Name:     name,
		ToolName: toolName,
		Args:     args,
	})
	if err != nil {
		return true, args, "", err // allow on error to avoid blocking
	}
	if resp.Error != nil {
		return true, args, "", fmt.Errorf("interceptor before: %s", resp.Error.Message)
	}
	var result InterceptorBeforeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return true, args, "", fmt.Errorf("unmarshal interceptor before: %w", err)
	}
	if result.Args != nil {
		return result.Allow, result.Args, result.Preview, nil
	}
	return result.Allow, args, result.Preview, nil
}

// InterceptAfter sends an interceptor/after request to the extension.
func (h *Host) InterceptAfter(ctx context.Context, name, toolName string, details any) (any, error) {
	resp, err := h.request(ctx, MethodInterceptorAfter, InterceptorAfterParams{
		Name:     name,
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

// TransformInput sends an inputTransformer/transform request to the extension.
func (h *Host) TransformInput(ctx context.Context, input string) (string, bool, error) {
	resp, err := h.request(ctx, MethodInputTransform, InputTransformParams{
		Input: input,
	})
	if err != nil {
		return input, false, err
	}
	if resp.Error != nil {
		return input, false, fmt.Errorf("input transform: %s", resp.Error.Message)
	}
	var result InputTransformResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return input, false, fmt.Errorf("unmarshal input transform: %w", err)
	}
	return result.Output, result.Handled, nil
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

// sendNotification sends a JSON-RPC notification (no ID, no response expected) to the extension.
func (h *Host) sendNotification(method string, params any) {
	paramsJSON, _ := json.Marshal(params)
	_ = h.send(&Message{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
	})
}
