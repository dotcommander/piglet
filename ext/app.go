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
//
// CAPABILITY GATE — before adding a new callback or field to App, answer:
//
//  1. Can this be a Tool?        → RegisterTool (agent-callable action)
//  2. Can this be a Command?     → RegisterCommand (user-invoked /slash)
//  3. Can this be a Shortcut?    → RegisterShortcut (keyboard binding)
//  4. Can this be a PromptSection? → RegisterPromptSection (system prompt injection)
//  5. Can this be an Interceptor? → RegisterInterceptor (before/after tool hook)
//
// Only add a new BindOption/callback when NONE of the above apply — typically
// for TUI-specific lifecycle that extensions cannot express through existing
// primitives (e.g. session swapping, background agent management).
// See ext/architecture_test.go for automated boundary enforcement.
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
	compactor      *Compactor
	statusSections []StatusSection
	extInfos       []ExtInfo

	// Runtime references (set via Bind)
	agent   AgentAPI
	actions []Action // queued actions for TUI to drain

	// Domain managers (set via Bind)
	sessions SessionManager
	models   ModelManager

	// Background agent callbacks
	runBackground       func(prompt string) error
	cancelBackground    func()
	isBackgroundRunning func() bool
}

// AgentAPI is the subset of *core.Agent that the extension runtime needs.
// Using an interface keeps ext/ from depending on agent implementation details.
type AgentAPI interface {
	Steer(msg core.Message)
	FollowUp(msg core.Message)
	SetTools(tools []core.Tool)
	SetModel(m core.Model)
	SetProvider(p core.StreamProvider)
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

// RegisterCompactor sets the conversation compactor.
// Only one compactor is active at a time (last-write-wins).
func (a *App) RegisterCompactor(c Compactor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.compactor = &c
}

// Compactor returns the registered compactor, or nil.
func (a *App) Compactor() *Compactor {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.compactor
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

// StatusSections returns all registered status sections.
func (a *App) StatusSections() []StatusSection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]StatusSection, len(a.statusSections))
	copy(out, a.statusSections)
	return out
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

// ---------------------------------------------------------------------------
// Runtime binding
// ---------------------------------------------------------------------------

// Bind wires the runtime references after the agent and session are created.
// Must be called before runtime methods (SendMessage, Model, etc.) are used.
func (a *App) Bind(agent AgentAPI, opts ...BindOption) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.agent = agent
	a.actions = a.actions[:0] // clear stale actions
	for _, opt := range opts {
		opt(a)
	}
}

// BindOption configures optional runtime callbacks for sync operations
// that cannot be expressed as fire-and-forget actions.
type BindOption func(*App)

// WithSessionManager binds the session manager.
func WithSessionManager(sm SessionManager) BindOption {
	return func(a *App) { a.sessions = sm }
}

// WithModelManager binds the model manager.
func WithModelManager(mm ModelManager) BindOption {
	return func(a *App) { a.models = mm }
}

// WithRunBackground sets the callback to start a background agent.
func WithRunBackground(fn func(prompt string) error) BindOption {
	return func(a *App) { a.runBackground = fn }
}

// WithCancelBackground sets the callback to cancel the running background agent.
func WithCancelBackground(fn func()) BindOption {
	return func(a *App) { a.cancelBackground = fn }
}

// WithIsBackgroundRunning sets the callback to check if a background agent is active.
func WithIsBackgroundRunning(fn func() bool) BindOption {
	return func(a *App) { a.isBackgroundRunning = fn }
}


// PendingActions returns and clears all queued actions.
func (a *App) PendingActions() []Action {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.actions
	a.actions = nil
	return out
}

func (a *App) enqueue(action Action) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, action)
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

// SetProvider swaps the agent's streaming provider.
func (a *App) SetProvider(p core.StreamProvider) {
	a.mu.RLock()
	agent := a.agent
	a.mu.RUnlock()
	if agent != nil {
		agent.SetProvider(p)
	}
}

// Notify sends a notification to the TUI.
func (a *App) Notify(msg string) {
	a.enqueue(ActionNotify{Message: msg})
}

// SetStatus updates a status bar widget.
func (a *App) SetStatus(key, text string) {
	a.enqueue(ActionSetStatus{Key: key, Text: text})
}

// ShowMessage displays a message in the TUI.
func (a *App) ShowMessage(text string) {
	a.enqueue(ActionShowMessage{Text: text})
}

// RequestQuit signals the TUI to quit.
func (a *App) RequestQuit() {
	a.enqueue(ActionQuit{})
}

// ShowPicker shows a picker/modal in the TUI.
func (a *App) ShowPicker(title string, items []PickerItem, onSelect func(PickerItem)) {
	a.enqueue(ActionShowPicker{Title: title, Items: items, OnSelect: onSelect})
}

// ---------------------------------------------------------------------------
// Session domain methods (backed by SessionManager)
// ---------------------------------------------------------------------------

// Sessions returns all sessions, newest first.
func (a *App) Sessions() ([]SessionSummary, error) {
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
	if sm == nil {
		return nil, fmt.Errorf("sessions not configured")
	}
	return sm.List()
}

// LoadSession opens a session by path and enqueues a swap.
func (a *App) LoadSession(path string) error {
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
	if sm == nil {
		return fmt.Errorf("sessions not configured")
	}
	sess, err := sm.Load(path)
	if err != nil {
		return err
	}
	a.enqueue(ActionSwapSession{Session: sess})
	return nil
}

// ForkSession forks the current session into a new branch.
func (a *App) ForkSession() (string, int, error) {
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
	if sm == nil {
		return "", 0, fmt.Errorf("no active session")
	}
	parentID, forked, count, err := sm.Fork()
	if err != nil {
		return "", 0, err
	}
	a.enqueue(ActionSwapSession{Session: forked})
	return parentID, count, nil
}

// SetSessionTitle updates the current session's title.
func (a *App) SetSessionTitle(title string) error {
	a.mu.RLock()
	sm := a.sessions
	a.mu.RUnlock()
	if sm == nil {
		return fmt.Errorf("no active session")
	}
	return sm.SetTitle(title)
}

// ---------------------------------------------------------------------------
// Model domain methods (backed by ModelManager)
// ---------------------------------------------------------------------------

// AvailableModels returns all registered models.
func (a *App) AvailableModels() []core.Model {
	a.mu.RLock()
	mm := a.models
	a.mu.RUnlock()
	if mm == nil {
		return nil
	}
	return mm.Available()
}

// SwitchModel activates a model by its "provider/id" key.
// Updates the agent's model and provider, and enqueues a status update.
func (a *App) SwitchModel(id string) error {
	a.mu.RLock()
	mm := a.models
	agent := a.agent
	a.mu.RUnlock()
	if mm == nil {
		return fmt.Errorf("model manager not configured")
	}
	mod, prov, err := mm.Switch(id)
	if err != nil {
		return err
	}
	if agent != nil {
		agent.SetModel(mod)
		agent.SetProvider(prov)
	}
	a.enqueue(ActionSetStatus{Key: "model", Text: mod.DisplayName()})
	return nil
}

// SyncModels updates the model catalog from an external source.
func (a *App) SyncModels() (int, error) {
	a.mu.RLock()
	mm := a.models
	a.mu.RUnlock()
	if mm == nil {
		return 0, fmt.Errorf("model manager not configured")
	}
	return mm.Sync()
}

// RunBackground starts a background agent with the given prompt.
// Returns an error if not bound or if a background agent is already running.
func (a *App) RunBackground(prompt string) error {
	a.mu.RLock()
	fn := a.runBackground
	a.mu.RUnlock()
	if fn == nil {
		return fmt.Errorf("background agent not available")
	}
	return fn(prompt)
}

// CancelBackground stops the running background agent. No-op if not bound or not running.
func (a *App) CancelBackground() {
	a.mu.RLock()
	fn := a.cancelBackground
	a.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// IsBackgroundRunning returns whether a background agent is currently active.
func (a *App) IsBackgroundRunning() bool {
	a.mu.RLock()
	fn := a.isBackgroundRunning
	a.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return false
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
