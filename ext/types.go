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
}

// Shortcut is a keyboard shortcut registered by an extension.
type Shortcut struct {
	Key         string // e.g. "ctrl+g"
	Description string
	Handler     func(app *App) error
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
// Prompt sections
// ---------------------------------------------------------------------------

// PromptSection is a block of text injected into the system prompt.
// Extensions use this to add instructions, guidelines, or context.
type PromptSection struct {
	Title   string // section heading (e.g. "Code Style")
	Content string // markdown content
	Order   int    // lower = earlier in prompt; default 0
}

// ---------------------------------------------------------------------------
// Provider registration
// ---------------------------------------------------------------------------

// ProviderConfig registers a custom LLM provider from an extension.
type ProviderConfig struct {
	BaseURL string
	APIKey  string
	API     core.API
	Models  []core.Model
	Headers map[string]string
}
