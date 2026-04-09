package ext

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/dotcommander/piglet/core"
)

// ---------------------------------------------------------------------------
// Registration (called during Extension init)
// ---------------------------------------------------------------------------

// RegisterTool adds a tool. Overwrites if name already exists.
// Panics with a descriptive message if t is nil, has empty Name, or nil Execute.
func (a *App) RegisterTool(t *ToolDef) {
	if t == nil {
		panic("ext: RegisterTool called with nil *ToolDef")
	}
	if t.Name == "" {
		panic("ext: RegisterTool called with empty Name")
	}
	if t.Execute == nil {
		panic("ext: RegisterTool called with nil Execute function")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tools[t.Name] = t
}

// RegisterCommand adds a slash command.
// Panics with a descriptive message if c is nil, has empty Name, or nil Handler.
func (a *App) RegisterCommand(c *Command) {
	if c == nil {
		panic("ext: RegisterCommand called with nil *Command")
	}
	if c.Name == "" {
		panic("ext: RegisterCommand called with empty Name")
	}
	if c.Handler == nil {
		panic("ext: RegisterCommand called with nil Handler")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.commands[c.Name] = c
}

// RegisterShortcut adds a keyboard shortcut.
func (a *App) RegisterShortcut(s *Shortcut) {
	if s == nil {
		panic("ext: RegisterShortcut called with nil *Shortcut")
	}
	if s.Key == "" {
		panic("ext: RegisterShortcut called with empty Key")
	}
	if s.Handler == nil {
		panic("ext: RegisterShortcut called with nil Handler")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.shortcuts[s.Key] = s
}

// RegisterInterceptor adds a tool interceptor. Sorted by priority descending.
func (a *App) RegisterInterceptor(i Interceptor) {
	if i.Name == "" {
		panic("ext: RegisterInterceptor called with empty Name")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.interceptors = append(a.interceptors, i)
	slices.SortFunc(a.interceptors, func(x, y Interceptor) int { return cmp.Compare(y.Priority, x.Priority) })
}

// RegisterMessageHook adds a hook that runs before user messages reach the LLM.
// Sorted by priority ascending (lower = earlier).
func (a *App) RegisterMessageHook(h MessageHook) {
	if h.Name == "" {
		panic("ext: RegisterMessageHook called with empty Name")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messageHooks = append(a.messageHooks, h)
	slices.SortFunc(a.messageHooks, func(x, y MessageHook) int { return cmp.Compare(x.Priority, y.Priority) })
}

// RegisterInputTransformer adds a transformer that intercepts user input before the agent.
// Sorted by priority ascending (lower = earlier).
func (a *App) RegisterInputTransformer(t InputTransformer) {
	if t.Name == "" {
		panic("ext: RegisterInputTransformer called with empty Name")
	}
	if t.Transform == nil {
		panic("ext: RegisterInputTransformer called with nil Transform")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inputTransformers = append(a.inputTransformers, t)
	slices.SortFunc(a.inputTransformers, func(x, y InputTransformer) int { return cmp.Compare(x.Priority, y.Priority) })
}

// RunInputTransformers executes all input transformers in priority order.
// Returns the (possibly modified) input text and whether the input was consumed.
func (a *App) RunInputTransformers(ctx context.Context, input string) (string, bool, error) {
	a.mu.RLock()
	transformers := make([]InputTransformer, len(a.inputTransformers))
	copy(transformers, a.inputTransformers)
	a.mu.RUnlock()

	current := input
	for _, t := range transformers {
		result, handled, err := t.Transform(ctx, current)
		if err != nil {
			return "", false, fmt.Errorf("input transformer %s: %w", t.Name, err)
		}
		if handled {
			return result, true, nil
		}
		current = result
	}
	return current, false, nil
}

// RunMessageHooks executes all message hooks in priority order.
// Returns collected non-empty context strings for ephemeral injection.
func (a *App) RunMessageHooks(ctx context.Context, msg string) ([]string, error) {
	a.mu.RLock()
	hooks := make([]MessageHook, len(a.messageHooks))
	copy(hooks, a.messageHooks)
	a.mu.RUnlock()

	var injections []string
	for _, h := range hooks {
		if h.OnMessage == nil {
			continue
		}
		extra, err := h.OnMessage(ctx, msg)
		if err != nil {
			return nil, fmt.Errorf("message hook %s: %w", h.Name, err)
		}
		if extra != "" {
			injections = append(injections, extra)
		}
	}
	return injections, nil
}

// RegisterRenderer adds a custom message type renderer.
func (a *App) RegisterRenderer(typ string, r Renderer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.renderers[typ] = r
}

// RegisterPromptSection adds a section to the system prompt.
func (a *App) RegisterPromptSection(s PromptSection) {
	if s.Title == "" {
		panic("ext: RegisterPromptSection called with empty Title")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.promptSections = append(a.promptSections, s)
	slices.SortFunc(a.promptSections, func(x, y PromptSection) int { return cmp.Compare(x.Order, y.Order) })
}

// RegisterCompactor sets the conversation compactor.
// Only one compactor is active at a time (last-write-wins).
func (a *App) RegisterCompactor(c Compactor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.compactor = &c
}

// RegisterStatusSection adds a status bar section.
// Overwrites if a section with the same key already exists.
func (a *App) RegisterStatusSection(s StatusSection) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Replace existing with same key
	for i, existing := range a.statusSections {
		if existing.Key == s.Key {
			a.statusSections[i] = s
			return
		}
	}
	a.statusSections = append(a.statusSections, s)
}

// StreamProviderFactory creates a StreamProvider configured for a specific model.
// Registered per API type; called each time the agent selects a model served by that API.
type StreamProviderFactory func(model core.Model) core.StreamProvider

// RegisterStreamProvider registers a factory that creates StreamProviders for the given API type.
func (a *App) RegisterStreamProvider(api string, factory StreamProviderFactory) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.streamProviders[api] = factory
}

// RegisterExtInfo records metadata about a loaded extension.
func (a *App) RegisterExtInfo(info ExtInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.extInfos = append(a.extInfos, info)
}
