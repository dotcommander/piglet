package ext

import (
	"context"
	"fmt"
	"github.com/dotcommander/piglet/core"
	"sort"
	"sync"
)

// App is the single extension API surface. Extensions receive this in their
// factory function and call methods to register tools, commands, etc.
//
// After registration, Bind() wires the runtime references (agent, session)
// so runtime methods like SendMessage() and Model() work.
type App struct {
	mu sync.RWMutex
	cwd string

	// Registration state
	tools          map[string]*ToolDef
	commands       map[string]*Command
	shortcuts      map[string]*Shortcut
	interceptors   []Interceptor
	renderers      map[string]Renderer
	providers      map[string]*ProviderConfig
	promptSections []PromptSection
	extInfos       []ExtInfo

	// Runtime references (set via Bind)
	agent       AgentAPI
	notify      func(msg string)
	status      func(key, text string)
	showMessage func(text string)
	requestQuit func()
	showPicker  func(title string, items []PickerItem, onSelect func(PickerItem))
}

// AgentAPI is the subset of *core.Agent that the extension runtime needs.
// Using an interface keeps ext/ from depending on agent implementation details.
type AgentAPI interface {
	Steer(msg core.Message)
	FollowUp(msg core.Message)
	SetTools(tools []core.Tool)
	SetModel(m core.Model)
	Messages() []core.Message
	IsRunning() bool
	StepMode() bool
	SetStepMode(on bool)
	SetMessages(msgs []core.Message)
}

// NewApp creates an extension App for the given working directory.
func NewApp(cwd string) *App {
	return &App{
		cwd:       cwd,
		tools:     make(map[string]*ToolDef),
		commands:  make(map[string]*Command),
		shortcuts: make(map[string]*Shortcut),
		renderers: make(map[string]Renderer),
		providers: make(map[string]*ProviderConfig),
	}
}

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

// ExtInfos returns metadata about all loaded extensions.
func (a *App) ExtInfos() []ExtInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]ExtInfo, len(a.extInfos))
	copy(out, a.extInfos)
	return out
}

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
	a.mu.RLock()
	defer a.mu.RUnlock()

	tools := make([]core.Tool, 0, len(a.tools))
	for _, td := range a.tools {
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

// ---------------------------------------------------------------------------
// Runtime binding
// ---------------------------------------------------------------------------

// Bind wires the runtime references after the agent and session are created.
// Must be called before runtime methods (SendMessage, Model, etc.) are used.
func (a *App) Bind(agent AgentAPI, opts ...BindOption) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.agent = agent
	for _, opt := range opts {
		opt(a)
	}
}

// BindOption configures optional runtime callbacks.
type BindOption func(*App)

// WithNotify sets the notification callback for the TUI.
func WithNotify(fn func(msg string)) BindOption {
	return func(a *App) { a.notify = fn }
}

// WithStatus sets the status bar callback for the TUI.
func WithStatus(fn func(key, text string)) BindOption {
	return func(a *App) { a.status = fn }
}

// WithShowMessage sets the callback to display a message in the TUI.
func WithShowMessage(fn func(text string)) BindOption {
	return func(a *App) { a.showMessage = fn }
}

// WithRequestQuit sets the callback to request the TUI to quit.
func WithRequestQuit(fn func()) BindOption {
	return func(a *App) { a.requestQuit = fn }
}

// WithShowPicker sets the callback to show a picker/modal in the TUI.
func WithShowPicker(fn func(title string, items []PickerItem, onSelect func(PickerItem))) BindOption {
	return func(a *App) { a.showPicker = fn }
}

// ---------------------------------------------------------------------------
// Runtime methods (available after Bind)
// ---------------------------------------------------------------------------

// CWD returns the working directory.
func (a *App) CWD() string { return a.cwd }

// SendMessage queues a user message for the agent as a follow-up.
func (a *App) SendMessage(content string) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.FollowUp(&core.UserMessage{Content: content})
	}
}

// Steer injects a steering message that interrupts the current turn.
func (a *App) Steer(content string) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.Steer(&core.UserMessage{Content: content})
	}
}

// Model returns the current model. Returns zero value if not bound.
func (a *App) Model() core.Model {
	// Agent doesn't expose model directly; this would need a ModelGetter interface
	// or the model stored on App. For now return zero.
	return core.Model{}
}

// SetModel updates the agent's model.
func (a *App) SetModel(m core.Model) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.SetModel(m)
	}
}

// Notify sends a notification to the TUI. No-op if not bound.
func (a *App) Notify(msg string) {
	a.mu.RLock()
	fn := a.notify
	a.mu.RUnlock()
	if fn != nil {
		fn(msg)
	}
}

// SetStatus updates a status bar widget. No-op if not bound.
func (a *App) SetStatus(key, text string) {
	a.mu.RLock()
	fn := a.status
	a.mu.RUnlock()
	if fn != nil {
		fn(key, text)
	}
}

// ShowMessage displays a message in the TUI. No-op if not bound.
func (a *App) ShowMessage(text string) {
	a.mu.RLock()
	fn := a.showMessage
	a.mu.RUnlock()
	if fn != nil {
		fn(text)
	}
}

// RequestQuit signals the TUI to quit. No-op if not bound.
func (a *App) RequestQuit() {
	a.mu.RLock()
	fn := a.requestQuit
	a.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// ShowPicker shows a picker/modal in the TUI. No-op if not bound.
func (a *App) ShowPicker(title string, items []PickerItem, onSelect func(PickerItem)) {
	a.mu.RLock()
	fn := a.showPicker
	a.mu.RUnlock()
	if fn != nil {
		fn(title, items, onSelect)
	}
}

// ConversationMessages returns a snapshot of the conversation history.
func (a *App) ConversationMessages() []core.Message {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		return agent.Messages()
	}
	return nil
}

// SetConversationMessages replaces the conversation history.
func (a *App) SetConversationMessages(msgs []core.Message) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.SetMessages(msgs)
	}
}

// ToggleStepMode toggles step mode and returns the new state.
func (a *App) ToggleStepMode() bool {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent == nil {
		return false
	}
	on := !agent.StepMode()
	agent.SetStepMode(on)
	return on
}

// ---------------------------------------------------------------------------
// Interceptor chain
// ---------------------------------------------------------------------------

func (a *App) wrapWithInterceptors(toolName string, execute core.ToolExecuteFn) core.ToolExecuteFn {
	if len(a.interceptors) == 0 {
		return execute
	}

	// Snapshot interceptors at registration time
	interceptors := make([]Interceptor, len(a.interceptors))
	copy(interceptors, a.interceptors)

	return func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
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
