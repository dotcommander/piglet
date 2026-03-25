package ext

import (
	"github.com/dotcommander/piglet/core"
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
//  1. Can this be a Tool?         → RegisterTool (agent-callable action)
//  2. Can this be a Command?      → RegisterCommand (user-invoked /slash)
//  3. Can this be a Shortcut?     → RegisterShortcut (keyboard binding)
//  4. Can this be a PromptSection? → RegisterPromptSection (system prompt injection)
//  5. Can this be an Interceptor?  → RegisterInterceptor (before/after tool hook)
//  6. Can this be a MessageHook?   → RegisterMessageHook (before user message reaches LLM)
//
// Only add a new BindOption/callback when NONE of the above apply — typically
// for TUI-specific lifecycle that extensions cannot express through existing
// primitives (e.g. session swapping, background agent management).
// See ext/architecture_test.go for automated boundary enforcement.
type App struct {
	mu  sync.RWMutex
	cwd string

	// Registration state
	tools          map[string]*ToolDef
	commands       map[string]*Command
	shortcuts      map[string]*Shortcut
	interceptors   []Interceptor
	messageHooks   []MessageHook
	renderers      map[string]Renderer
	providers      map[string]*ProviderConfig
	promptSections []PromptSection
	compactor      *Compactor
	statusSections []StatusSection
	eventHandlers  []EventHandler
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

// AgentReader provides read-only access to agent state.
type AgentReader interface {
	Messages() []core.Message
	StepMode() bool
	Provider() core.StreamProvider
	System() string
}

// AgentWriter provides mutation access to the agent.
type AgentWriter interface {
	Steer(msg core.Message)
	FollowUp(msg core.Message)
	SetModel(m core.Model)
	SetProvider(p core.StreamProvider)
	SetMessages(msgs []core.Message)
	SetStepMode(on bool)
}

// AgentAPI is the subset of *core.Agent that the extension runtime needs.
// Using an interface keeps ext/ from depending on agent implementation details.
//
// Removed from interface (used only via *core.Agent in tui/ and cmd/):
//   - SetTools, SetTurnContext, IsRunning
type AgentAPI interface {
	AgentReader
	AgentWriter
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
// Bind wiring
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

// ---------------------------------------------------------------------------
// Action queue
// ---------------------------------------------------------------------------

// PendingActions returns and clears all queued actions.
func (a *App) PendingActions() []Action {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.actions
	a.actions = nil
	return out
}

// EnqueueAction adds an action to the pending queue.
// Used by the TUI to re-enqueue results from ActionRunAsync.
func (a *App) EnqueueAction(action Action) {
	a.enqueue(action)
}

func (a *App) enqueue(action Action) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, action)
}
