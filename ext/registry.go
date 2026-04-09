package ext

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
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

// UnregisterExtension removes all registrations associated with the named extension.
// Used by the supervisor when restarting a crashed extension process.
func (a *App) UnregisterExtension(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Find the extension's info to know what was registered
	var info *ExtInfo
	for i := range a.extInfos {
		if a.extInfos[i].Name == name {
			info = &a.extInfos[i]
			break
		}
	}
	if info == nil {
		return
	}

	// Remove tools
	for _, t := range info.Tools {
		delete(a.tools, t)
	}

	// Remove commands
	for _, c := range info.Commands {
		delete(a.commands, c)
	}

	// Remove shortcuts
	for _, k := range info.Shortcuts {
		delete(a.shortcuts, k)
	}

	nameSet := func(names []string) map[string]bool {
		m := make(map[string]bool, len(names))
		for _, n := range names {
			m[n] = true
		}
		return m
	}

	interceptorNames := nameSet(info.Interceptors)
	handlerNames := nameSet(info.EventHandlers)
	hookNames := nameSet(info.MessageHooks)
	transformerNames := nameSet(info.InputTransformers)
	sectionTitles := nameSet(info.PromptSections)

	a.interceptors = slices.DeleteFunc(a.interceptors, func(ic Interceptor) bool {
		return interceptorNames[ic.Name]
	})
	a.eventHandlers = slices.DeleteFunc(a.eventHandlers, func(eh EventHandler) bool {
		return handlerNames[eh.Name]
	})
	a.messageHooks = slices.DeleteFunc(a.messageHooks, func(mh MessageHook) bool {
		return hookNames[mh.Name]
	})
	a.inputTransformers = slices.DeleteFunc(a.inputTransformers, func(it InputTransformer) bool {
		return transformerNames[it.Name]
	})
	a.promptSections = slices.DeleteFunc(a.promptSections, func(ps PromptSection) bool {
		return sectionTitles[ps.Title]
	})

	// Remove compactor if it belongs to this extension
	if a.compactor != nil && info.Compactor != "" && a.compactor.Name == info.Compactor {
		a.compactor = nil
	}

	// Remove stream providers
	for _, api := range info.StreamProviders {
		delete(a.streamProviders, api)
	}

	// Remove the ext info entry itself
	a.extInfos = slices.DeleteFunc(a.extInfos, func(ei ExtInfo) bool {
		return ei.Name == name
	})
}

// ---------------------------------------------------------------------------
// Inter-extension event bus
// ---------------------------------------------------------------------------

// Subscribe registers a callback for a topic. Returns an unsubscribe function.
// Callbacks run synchronously in the publisher's goroutine — keep them fast.
func (a *App) Subscribe(topic string, fn func(any)) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.eventBusSeq++
	id := a.eventBusSeq
	a.eventBus[topic] = append(a.eventBus[topic], eventSub{id: id, fn: fn})
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.eventBus[topic] = slices.DeleteFunc(a.eventBus[topic], func(s eventSub) bool { return s.id == id })
	}
}

// ---------------------------------------------------------------------------
// Interceptor chain
// ---------------------------------------------------------------------------

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
