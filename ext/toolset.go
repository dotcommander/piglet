package ext

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/dotcommander/piglet/core"
)

func (a *App) filterTools(pred func(*ToolDef) bool, mode ToolMode) []core.Tool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// unsafeMu serializes concurrency-unsafe tools within this tool set.
	// Each CoreTools()/BackgroundSafeTools() call gets its own mutex —
	// agents are independent and don't share tool execution locks.
	var unsafeMu sync.Mutex

	// Collect matching defs first, then sort by name for deterministic order.
	// Stable tool ordering prevents prompt cache invalidation on every API call
	// (Go map iteration is non-deterministic).
	defs := make([]*ToolDef, 0, len(a.tools))
	for _, td := range a.tools {
		if pred != nil && !pred(td) {
			continue
		}
		if a.toolFilter != nil && !a.toolFilter(td.Name) {
			continue
		}
		defs = append(defs, td)
	}
	slices.SortFunc(defs, func(a, b *ToolDef) int {
		return cmp.Compare(a.Name, b.Name)
	})

	tools := make([]core.Tool, 0, len(defs))
	for _, td := range defs {
		schema := td.ToolSchema
		if td.Deferred && mode == ToolModeCompact && !a.activatedTools[td.Name] {
			// Send name+description only; parameters are nil → minimal schema in API.
			schema.Parameters = nil
		}
		execute := a.wrapWithInterceptors(td.Name, td.Execute)
		if td.ConcurrencySafe != nil {
			execute = wrapConcurrencySafe(execute, td.ConcurrencySafe, &unsafeMu)
		}
		if td.InterruptBehavior == InterruptBlock {
			execute = wrapInterruptBlock(execute)
		}
		tools = append(tools, core.Tool{
			ToolSchema: schema,
			Execute:    execute,
		})
	}
	return tools
}

func wrapConcurrencySafe(
	execute core.ToolExecuteFn,
	check func(args map[string]any) bool,
	mu *sync.Mutex,
) core.ToolExecuteFn {
	return func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
		if !check(args) {
			mu.Lock()
			defer mu.Unlock()
		}
		return execute(ctx, id, args)
	}
}

func wrapInterruptBlock(execute core.ToolExecuteFn) core.ToolExecuteFn {
	return func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
		return execute(context.WithoutCancel(ctx), id, args)
	}
}
