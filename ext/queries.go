package ext

import (
	"cmp"
	"slices"

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
// Wraps each tool's Execute with the interceptor chain. Uses current ToolMode.
func (a *App) CoreTools() []core.Tool {
	a.mu.RLock()
	mode := a.toolMode
	a.mu.RUnlock()
	return a.filterTools(nil, mode)
}

// CoreToolsForModel returns tools with deferred handling based on the given mode.
func (a *App) CoreToolsForModel(mode ToolMode) []core.Tool {
	return a.filterTools(nil, mode)
}

// BackgroundSafeTools returns core.Tool slice filtered to tools marked BackgroundSafe.
func (a *App) BackgroundSafeTools() []core.Tool {
	a.mu.RLock()
	mode := a.toolMode
	a.mu.RUnlock()
	return a.filterTools(func(td *ToolDef) bool { return td.BackgroundSafe }, mode)
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
