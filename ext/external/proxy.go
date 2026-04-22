package external

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// proxyToolExecute returns a ToolExecuteFn that proxies to the extension process.
func proxyToolExecute(h *Host, toolName string) core.ToolExecuteFn {
	return func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
		result, err := h.ExecuteTool(ctx, id, toolName, args)
		if err != nil {
			return nil, fmt.Errorf("ext %s tool %s: %w", h.Name(), toolName, err)
		}

		content := wireToCore(result.Content)
		content = ensureCodedErrorPrefix(content, result.ErrorCode, result.ErrorHint)
		return &core.ToolResult{Content: content}, nil
	}
}

// proxyCommandExecute returns a command handler that proxies to the extension.
func proxyCommandExecute(h *Host, cmdName string) func(args string, app *ext.App) error {
	return func(args string, app *ext.App) error {
		return h.ExecuteCommand(h.ctx, cmdName, args)
	}
}

// proxyInterceptorBeforeWithPreview returns a paired Before + Preview function.
// The Before captures the preview from the extension's response; Preview returns it.
func proxyInterceptorBeforeWithPreview(h *Host, name string) (
	func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error),
	func(ctx context.Context, toolName string, args map[string]any) string,
) {
	var lastPreview atomic.Value // stores string

	before := func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
		select {
		case <-h.Closed():
			return true, args, nil
		default:
		}
		allow, modArgs, preview, err := h.InterceptBefore(ctx, name, toolName, args)
		if preview != "" {
			lastPreview.Store(preview)
		}
		return allow, modArgs, err
	}

	preview := func(_ context.Context, _ string, _ map[string]any) string {
		if v, ok := lastPreview.Load().(string); ok {
			return v
		}
		return ""
	}

	return before, preview
}

// proxyInterceptorAfter returns an After function that proxies to the extension.
func proxyInterceptorAfter(h *Host, name string) func(ctx context.Context, toolName string, details any) (any, error) {
	return func(ctx context.Context, toolName string, details any) (any, error) {
		select {
		case <-h.Closed():
			return details, nil // host dead — pass through
		default:
		}
		return h.InterceptAfter(ctx, name, toolName, details)
	}
}

// proxyEventFilter returns a Filter function that checks event type names.
// nil events slice means accept all events.
func proxyEventFilter(events []string) func(core.Event) bool {
	if len(events) == 0 {
		return nil // nil = accept all
	}
	return func(evt core.Event) bool {
		typeName := eventTypeName(evt)
		return slices.Contains(events, typeName)
	}
}

// proxyEventHandle returns a Handle function that dispatches events to the extension.
// Wraps in ActionRunAsync since extension calls may be slow (e.g. LLM calls for autotitle).
func proxyEventHandle(h *Host) func(ctx context.Context, evt core.Event) ext.Action {
	return func(ctx context.Context, evt core.Event) ext.Action {
		typeName := eventTypeName(evt)
		data, _ := json.Marshal(evt)

		return ext.ActionRunAsync{Fn: func() ext.Action {
			ar, err := h.DispatchEvent(ctx, typeName, data)
			if err != nil {
				slog.Debug("event dispatch error", "ext", h.Name(), "err", err)
				return nil
			}
			return actionResultToAction(ar)
		}}
	}
}

// proxyShortcutHandle returns a Handler function that proxies to the extension.
func proxyShortcutHandle(h *Host, key string) func(app *ext.App) (ext.Action, error) {
	return func(app *ext.App) (ext.Action, error) {
		ar, err := h.HandleShortcut(h.ctx, key)
		if err != nil {
			return nil, fmt.Errorf("ext %s shortcut %s: %w", h.Name(), key, err)
		}
		return actionResultToAction(ar), nil
	}
}

// proxyMessageHook returns an OnMessage function that proxies to the extension.
func proxyMessageHook(h *Host) func(ctx context.Context, msg string) (string, error) {
	return func(ctx context.Context, msg string) (string, error) {
		return h.OnMessage(ctx, msg)
	}
}

// proxyInputTransform returns a Transform function that proxies to the extension.
func proxyInputTransform(h *Host) func(ctx context.Context, input string) (string, bool, error) {
	return func(ctx context.Context, input string) (string, bool, error) {
		select {
		case <-h.Closed():
			return input, false, nil // host dead — pass through
		default:
		}
		return h.TransformInput(ctx, input)
	}
}

// proxyCompactExecute returns a Compact function that proxies to the extension.
func proxyCompactExecute(h *Host) func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
	return func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		// Serialize messages with type discriminator
		wire := make([]CompactMessage, 0, len(msgs))
		for _, m := range msgs {
			var msgType string
			switch m.(type) {
			case *core.UserMessage:
				msgType = "user"
			case *core.AssistantMessage:
				msgType = "assistant"
			case *core.ToolResultMessage:
				msgType = "tool_result"
			default:
				continue
			}
			data, err := json.Marshal(m)
			if err != nil {
				continue
			}
			wire = append(wire, CompactMessage{Type: msgType, Data: data})
		}

		result, err := h.ExecuteCompact(ctx, wire)
		if err != nil {
			return nil, err
		}

		// Deserialize back to core.Message
		return compactWireToCore(result), nil
	}
}
