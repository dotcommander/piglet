package ext

import (
	"github.com/dotcommander/piglet/core"
	"sort"
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
	sort.Strings(names)
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

// ToolDefs returns all registered tool definitions.
func (a *App) ToolDefs() []*ToolDef {
	a.mu.RLock()
	defer a.mu.RUnlock()
	defs := make([]*ToolDef, 0, len(a.tools))
	for _, d := range a.tools {
		defs = append(defs, d)
	}
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

	tools := make([]core.Tool, 0, len(a.tools))
	for _, td := range a.tools {
		if pred != nil && !pred(td) {
			continue
		}
		tools = append(tools, core.Tool{
			ToolSchema: td.ToolSchema,
			Execute:    a.wrapWithInterceptors(td.Name, td.Execute),
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
