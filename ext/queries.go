package ext

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/dotcommander/piglet/core"
)

// ---------------------------------------------------------------------------
// Query registration state
// ---------------------------------------------------------------------------

// Tools returns the names of all registered tools.
func (a *App) Tools() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	names := make([]string, 0, len(a.tools))
	for name := range a.tools {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// FindTool looks up a tool by name and returns a core.Tool with interceptors applied.
// Returns nil if no tool with that name is registered.
func (a *App) FindTool(name string) *core.Tool {
	a.mu.RLock()
	td, ok := a.tools[name]
	a.mu.RUnlock()
	if !ok {
		return nil
	}
	return &core.Tool{
		ToolSchema: td.ToolSchema,
		Execute:    a.wrapWithInterceptors(td.Name, td.Execute),
	}
}

// ToolDefs returns all registered tool definitions, sorted by name.
func (a *App) ToolDefs() []*ToolDef {
	a.mu.RLock()
	defer a.mu.RUnlock()
	defs := make([]*ToolDef, 0, len(a.tools))
	for _, d := range a.tools {
		defs = append(defs, d)
	}
	slices.SortFunc(defs, func(a, b *ToolDef) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return defs
}

// CoreTools converts registered ToolDefs into core.Tool slice for the agent.
// Wraps each tool's Execute with the interceptor chain.
func (a *App) CoreTools() []core.Tool {
	return a.filterTools(nil)
}

// BackgroundSafeTools returns core.Tool slice filtered to tools marked BackgroundSafe.
func (a *App) BackgroundSafeTools() []core.Tool {
	return a.filterTools(func(td *ToolDef) bool { return td.BackgroundSafe })
}

func (a *App) filterTools(pred func(*ToolDef) bool) []core.Tool {
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
		defs = append(defs, td)
	}
	slices.SortFunc(defs, func(a, b *ToolDef) int {
		return cmp.Compare(a.Name, b.Name)
	})

	tools := make([]core.Tool, 0, len(defs))
	for _, td := range defs {
		schema := td.ToolSchema
		if td.Deferred {
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

// Commands returns all registered commands.
func (a *App) Commands() map[string]*Command {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]*Command, len(a.commands))
	for k, v := range a.commands {
		out[k] = v
	}
	return out
}

// Shortcuts returns all registered shortcuts.
func (a *App) Shortcuts() map[string]*Shortcut {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]*Shortcut, len(a.shortcuts))
	for k, v := range a.shortcuts {
		out[k] = v
	}
	return out
}

// Renderers returns all registered renderers.
func (a *App) Renderers() map[string]Renderer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]Renderer, len(a.renderers))
	for k, v := range a.renderers {
		out[k] = v
	}
	return out
}

// PromptSections returns all registered prompt sections, sorted by order.
func (a *App) PromptSections() []PromptSection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]PromptSection, len(a.promptSections))
	copy(out, a.promptSections)
	return out
}

// Providers returns all registered provider configs.
func (a *App) Providers() map[string]*ProviderConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]*ProviderConfig, len(a.providers))
	for k, v := range a.providers {
		out[k] = v
	}
	return out
}

// StreamProvider returns a provider for the given API type and model, if a factory is registered.
func (a *App) StreamProvider(api string, model core.Model) (core.StreamProvider, bool) {
	a.mu.RLock()
	factory, ok := a.streamProviders[api]
	a.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return factory(model), true
}

// StatusSections returns all registered status sections.
func (a *App) StatusSections() []StatusSection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]StatusSection, len(a.statusSections))
	copy(out, a.statusSections)
	return out
}

// Compactor returns the registered compactor, or nil.
func (a *App) Compactor() *Compactor {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.compactor
}

// ExtInfos returns metadata about all loaded extensions.
func (a *App) ExtInfos() []ExtInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]ExtInfo, len(a.extInfos))
	copy(out, a.extInfos)
	return out
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
