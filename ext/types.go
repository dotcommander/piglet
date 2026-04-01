// Package ext provides the extension API for piglet.
// Extensions are plain functions that receive *App and register capabilities.
package ext

import (
	"context"
	"github.com/dotcommander/piglet/core"
)

// Extension is a function that registers tools, commands, interceptors, etc.
// Built-in tools and external extensions use the same signature.
type Extension func(app *App) error

// ---------------------------------------------------------------------------
// Tool definition
// ---------------------------------------------------------------------------

// InterruptBehavior controls how a tool reacts to steering (mid-turn user input).
type InterruptBehavior int

const (
	// InterruptCancel cancels the tool on steer and discards its result (default).
	InterruptCancel InterruptBehavior = iota
	// InterruptBlock keeps the tool running when steered; the steer is queued
	// for after the tool completes.
	InterruptBlock
)

// ToolDef extends core.ToolSchema with execution and optional UI rendering.
type ToolDef struct {
	core.ToolSchema

	// Execute runs the tool. Same signature as core.ToolExecuteFn.
	Execute func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error)

	// PromptHint is a one-liner injected into the system prompt.
	// Example: "Read file contents with line numbers"
	PromptHint string

	// PromptGuides are bullets injected into the system prompt guidelines.
	// Example: ["Use offset/limit for files >2000 lines", "Prefer grep to find content"]
	PromptGuides []string

	// BackgroundSafe marks this tool as safe for background agent use (read-only).
	BackgroundSafe bool

	// ConcurrencySafe returns true if this tool call is safe to run concurrently
	// with other tool calls. Return false for destructive/mutating operations.
	// nil means always safe (default: parallel execution).
	ConcurrencySafe func(args map[string]any) bool

	// InterruptBehavior controls how this tool reacts to steering.
	// InterruptBlock keeps the tool running; InterruptCancel (default) cancels it.
	InterruptBehavior InterruptBehavior

	// Deferred marks this tool as rarely used. Only name+description sent in API schemas.
	// Full schema available via tool_search.
	Deferred bool
}

// ---------------------------------------------------------------------------
// Commands and shortcuts
// ---------------------------------------------------------------------------

// Command is a slash command registered by an extension.
type Command struct {
	Name        string
	Description string
	Handler     func(args string, app *App) error
	Complete    func(prefix string) []string // tab completion; nil = no completion
	Immediate   bool                         // if true, executes during streaming without queuing
}

// Shortcut is a keyboard shortcut registered by an extension.
type Shortcut struct {
	Key         string // e.g. "ctrl+g"
	Description string
	Handler     func(app *App) (Action, error)
}

// ---------------------------------------------------------------------------
// Interceptors
// ---------------------------------------------------------------------------

// Interceptor wraps tool execution with before/after hooks.
// Higher priority runs first. Use priority 2000+ for security, 1000+ for logging.
type Interceptor struct {
	Name     string
	Priority int

	// Before is called before tool execution.
	// Return allow=false to block the tool call.
	// Return modified args to transform input.
	Before func(ctx context.Context, toolName string, args map[string]any) (allow bool, modifiedArgs map[string]any, err error)

	// After is called after tool execution.
	// Return modified result to transform output.
	After func(ctx context.Context, toolName string, result any) (any, error)
}

// ---------------------------------------------------------------------------
// Compactor
// ---------------------------------------------------------------------------

// Compactor controls conversation compaction (summarization to save tokens).
// Register one via App.RegisterCompactor. The Compact function receives all
// messages and returns the compacted set — giving full control over what to
// keep and how to summarize.
type Compactor struct {
	Name      string
	Threshold int // token threshold for auto-compact; 0 = use config default
	Compact   func(ctx context.Context, messages []core.Message) ([]core.Message, error)
}

// ---------------------------------------------------------------------------
// Status sections
// ---------------------------------------------------------------------------

// StatusSide determines which side of the status bar a section appears on.
type StatusSide int

const (
	StatusLeft StatusSide = iota
	StatusRight
)

// StatusSection defines an extensible status bar segment.
// Register via App.RegisterStatusSection. The TUI renders all registered
// sections sorted by Order within each side (left/right).
type StatusSection struct {
	Key   string     // unique key (e.g. "model", "tokens", "cost")
	Side  StatusSide // left or right side of status bar
	Order int        // lower = rendered first within the side
}

// Built-in status section keys.
const (
	StatusKeyApp          = "app"
	StatusKeyModel        = "model"
	StatusKeyMouse        = "mouse"
	StatusKeyBg           = "bg"
	StatusKeyTokens       = "tokens"
	StatusKeyCost         = "cost"
	StatusKeyQueue        = "queue"
	StatusKeyPromptBudget = "prompt-budget"
)

// ---------------------------------------------------------------------------
// Renderers
// ---------------------------------------------------------------------------

// Renderer renders a custom message type for display.
// expanded controls whether the message is shown in full or collapsed.
type Renderer func(message any, expanded bool) string

// PickerItem is an item in a picker/modal list.
// Used by commands to request a selection UI from the TUI.
type PickerItem struct {
	ID    string
	Label string
	Desc  string
}

// ---------------------------------------------------------------------------
// Message hooks
// ---------------------------------------------------------------------------

// MessageHook runs before a user message is sent to the LLM.
// Lower priority runs first (same convention as prompt sections).
type MessageHook struct {
	Name     string
	Priority int

	// OnMessage sees the user message before the LLM call.
	// Returns additional context to inject for this turn only (ephemeral).
	// Empty string = no injection. Error = abort the message.
	OnMessage func(ctx context.Context, msg string) (string, error)
}

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

// EventHandler reacts to agent lifecycle events (Observe primitive).
// Lower priority runs first. Handle must be fast (<50ms) — return
// ActionRunAsync for expensive work.
type EventHandler struct {
	Name     string
	Priority int

	// Filter limits which events this handler sees. nil = all events.
	Filter func(core.Event) bool

	// Handle processes the event. Returns an optional Action to enqueue.
	// Return nil for no action.
	Handle func(ctx context.Context, evt core.Event) Action
}

// ---------------------------------------------------------------------------
// Prompt sections
// ---------------------------------------------------------------------------

// PromptSection is a block of text injected into the system prompt.
// Extensions use this to add instructions, guidelines, or context.
type PromptSection struct {
	Title     string // section heading (e.g. "Code Style")
	Content   string // markdown content
	Order     int    // lower = earlier in prompt; default 0
	TokenHint int    // estimated token count; 0 = unknown
}

// ---------------------------------------------------------------------------
// Provider registration
// ---------------------------------------------------------------------------

// ExtInfo describes a loaded extension for /extensions listing.
type ExtInfo struct {
	Name            string   // human-readable name
	Version         string   // semver or empty
	Kind            string   // "builtin" or "external"
	Runtime         string   // "go", "bun", "node", "python", etc.
	Tools           []string // tool names registered by this extension
	Commands        []string // command names registered by this extension
	Interceptors    []string // interceptor names
	EventHandlers   []string // event handler names
	Shortcuts       []string // shortcut keys
	MessageHooks    []string // message hook names
	Compactor       string   // compactor name, empty if none
	PromptSections  []string // prompt section titles
	StreamProviders []string // stream provider API types (e.g. "openai")
}

// ProviderConfig registers a custom LLM provider from an extension.
type ProviderConfig struct {
	BaseURL string
	APIKey  string
	API     core.API
	Models  []core.Model
	Headers map[string]string
}
