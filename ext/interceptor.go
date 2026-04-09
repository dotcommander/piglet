package ext

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/dotcommander/piglet/core"
)

func (a *App) wrapWithInterceptors(toolName string, execute core.ToolExecuteFn) core.ToolExecuteFn {
	return func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
		// Evaluate interceptor chain at call time, not registration time.
		// This ensures interceptors registered after CoreTools() still fire.
		a.mu.RLock()
		if len(a.interceptors) == 0 {
			a.mu.RUnlock()
			return execute(ctx, id, args)
		}
		interceptors := make([]Interceptor, len(a.interceptors))
		copy(interceptors, a.interceptors)
		a.mu.RUnlock()

		// Before chain
		currentArgs := maps.Clone(args)
		for _, ic := range interceptors {
			if ic.Before == nil {
				continue
			}
			allow, modified, err := ic.Before(ctx, toolName, currentArgs)
			if err != nil {
				return nil, fmt.Errorf("interceptor %s before: %w", ic.Name, err)
			}
			if !allow {
				msg := fmt.Sprintf("blocked by interceptor: %s", ic.Name)
				if ic.Preview != nil {
					if preview := ic.Preview(ctx, toolName, currentArgs); preview != "" {
						msg = preview
					}
				}
				return &core.ToolResult{
					Content: []core.ContentBlock{core.TextContent{Text: msg}},
				}, nil
			}
			if modified != nil {
				currentArgs = modified
			}
		}

		// Execute
		result, err := execute(ctx, id, currentArgs)
		if err != nil {
			return result, err
		}

		// After chain
		var details any
		if result != nil {
			details = result.Details
		}
		for _, ic := range interceptors {
			if ic.After == nil {
				continue
			}
			modified, afterErr := ic.After(ctx, toolName, details)
			if afterErr != nil {
				slog.Debug("interceptor after error", "name", ic.Name, "tool", toolName, "err", afterErr)
				continue
			}
			details = modified
		}
		if result != nil {
			result.Details = details
		}

		return result, nil
	}
}
