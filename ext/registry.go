package ext

import (
	"context"
	"fmt"
	"github.com/dotcommander/piglet/core"
	"sort"
)

// ---------------------------------------------------------------------------
// Registration (called during Extension init)
// ---------------------------------------------------------------------------

// RegisterTool adds a tool. Overwrites if name already exists.
func (a *App) RegisterTool(t *ToolDef) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tools[t.Name] = t
}

// RegisterCommand adds a slash command.
func (a *App) RegisterCommand(c *Command) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.commands[c.Name] = c
}

// RegisterShortcut adds a keyboard shortcut.
func (a *App) RegisterShortcut(s *Shortcut) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.shortcuts[s.Key] = s
}

// RegisterInterceptor adds a tool interceptor. Sorted by priority descending.
func (a *App) RegisterInterceptor(i Interceptor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.interceptors = append(a.interceptors, i)
	sort.Slice(a.interceptors, func(x, y int) bool {
		return a.interceptors[x].Priority > a.interceptors[y].Priority
	})
}

// RegisterMessageHook adds a hook that runs before user messages reach the LLM.
// Sorted by priority ascending (lower = earlier).
func (a *App) RegisterMessageHook(h MessageHook) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messageHooks = append(a.messageHooks, h)
	sort.Slice(a.messageHooks, func(i, j int) bool {
		return a.messageHooks[i].Priority < a.messageHooks[j].Priority
	})
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
	a.mu.Lock()
	defer a.mu.Unlock()
	a.promptSections = append(a.promptSections, s)
	sort.Slice(a.promptSections, func(i, j int) bool {
		return a.promptSections[i].Order < a.promptSections[j].Order
	})
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

// RegisterProvider adds a custom LLM provider.
func (a *App) RegisterProvider(name string, cfg *ProviderConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.providers[name] = cfg
}

// RegisterExtInfo records metadata about a loaded extension.
func (a *App) RegisterExtInfo(info ExtInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.extInfos = append(a.extInfos, info)
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
		currentArgs := args
		for _, ic := range interceptors {
			if ic.Before == nil {
				continue
			}
			allow, modified, err := ic.Before(ctx, toolName, currentArgs)
			if err != nil {
				return nil, fmt.Errorf("interceptor %s before: %w", ic.Name, err)
			}
			if !allow {
				return &core.ToolResult{
					Content: []core.ContentBlock{core.TextContent{Text: fmt.Sprintf("blocked by interceptor: %s", ic.Name)}},
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
				// Log but don't fail
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
