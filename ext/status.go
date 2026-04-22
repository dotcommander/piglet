// Package-level status section types and keys.
package ext

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
	StatusKeyGuardrail    = "guardrail"
)
