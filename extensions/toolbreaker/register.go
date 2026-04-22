package toolbreaker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dotcommander/piglet/errfmt"
	sdk "github.com/dotcommander/piglet/sdk"
)

const (
	defaultLimit     = 3
	interceptorName  = "toolbreaker"
	eventHandlerName = "toolbreaker"
)

// Register wires the per-tool circuit breaker into the extension.
// Config key: max_tool_errors (int, default 3, 0 = disabled).
// After N consecutive errors, a tool's Before hook short-circuits with
// [error:TOOL_DISABLED]; EventToolEnd tracks success/failure.
func Register(e *sdk.Extension) {
	tracker := New()
	var limit int = defaultLimit

	e.OnInitAppend(func(x *sdk.Extension) {
		if vals, err := x.ConfigGet(context.Background(), "max_tool_errors"); err == nil {
			if v, ok := vals["max_tool_errors"]; ok {
				if n := toInt(v); n >= 0 { // 0 is a valid "disabled" value
					limit = n
				}
			}
		}
	})

	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     interceptorName,
		Priority: 900, // below safeguard (2000) and security checks — circuit breaker fires after safety
		Before: func(_ context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			if toolName == "" || limit <= 0 {
				return true, args, nil
			}
			if tracker.IsDisabled(toolName, limit) {
				return false, nil, nil
			}
			return true, args, nil
		},
		Preview: func(_ context.Context, toolName string, _ map[string]any) string {
			// Use the errfmt code constant for the prefix so the LLM pattern-matches it.
			return fmt.Sprintf(
				"[error:%s] %s disabled after %d consecutive errors\nhint: tool or its args may be broken; fix the issue and try again, or /clear to reset",
				errfmt.ToolErrToolDisabled, toolName, limit,
			)
		},
	})

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:   eventHandlerName,
		Events: []string{"EventToolEnd"},
		Handle: func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
			var evt struct {
				ToolName string `json:"ToolName"`
				IsError  bool   `json:"IsError"`
			}
			if err := json.Unmarshal(data, &evt); err != nil || evt.ToolName == "" {
				return nil
			}
			// Skip tools that are already disabled — their EventToolEnd comes
			// from the interceptor block (IsError=false), not actual execution.
			// Recording would falsely reset the breaker.
			if tracker.IsDisabled(evt.ToolName, limit) {
				return nil
			}
			if evt.IsError {
				tracker.RecordError(evt.ToolName)
			} else {
				tracker.RecordSuccess(evt.ToolName)
			}
			return nil
		},
	})
}

// toInt converts JSON-decoded numeric values (float64, int, int64) to int.
// Returns -1 for unrecognized types so callers can distinguish "not set" from 0.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return -1
}
