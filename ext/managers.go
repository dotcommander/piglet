package ext

import (
	"time"

	"github.com/dotcommander/piglet/core"
)

// EntryInfo is the ext-layer view of a session entry for display.
// Mirrors session.EntryInfo without importing session/.
type EntryInfo struct {
	ID        string
	ParentID  string
	Type      string
	Timestamp time.Time
	Children  int
}

// TreeNode is the ext-layer view of a full tree node for DAG rendering.
// Mirrors session.TreeNode without importing session/.
type TreeNode struct {
	ID           string
	ParentID     string
	Type         string
	Timestamp    time.Time
	Children     int
	OnActivePath bool
	Depth        int
	Preview      string
	Label        string
}

// SessionSummary is the ext-layer view of a session.
// Mirrors session.Summary without importing session/.
type SessionSummary struct {
	ID        string
	Path      string
	Title     string
	Model     string
	CWD       string
	CreatedAt time.Time
	Messages  int
	ParentID  string
}

// SessionManager provides session operations to commands and extensions.
// Implemented by the wiring layer (main.go/tui) which imports session/.
type SessionManager interface {
	// List returns all sessions, newest first.
	List() ([]SessionSummary, error)

	// Load opens a session by path.
	// Returns an opaque session handle for ActionSwapSession.
	Load(path string) (any, error)

	// Fork creates a branch of the current session.
	// Returns the parent short ID, forked session handle, message count, and any error.
	Fork() (parentID string, forked any, count int, err error)

	// Branch moves the current session's leaf to an earlier entry (in-place branching).
	// Returns the same session handle for refreshing the TUI/agent.
	Branch(entryID string) (session any, err error)

	// BranchWithSummary moves the leaf and writes a branch_summary entry.
	BranchWithSummary(entryID, summary string) (session any, err error)

	// EntryInfos returns info about entries on the current branch for display.
	EntryInfos() []EntryInfo

	// SetTitle updates the current session's title.
	SetTitle(title string) error

	// Title returns the current session's title (empty if not set).
	Title() string

	// AppendEntry writes a custom extension entry to the current session.
	// The kind should be namespaced (e.g., "ext:memory:facts").
	AppendEntry(kind string, data any) error

	// AppendCustomMessage writes a message that persists AND appears in Messages().
	// Role must be "user" or "assistant".
	AppendCustomMessage(role, content string) error

	// AppendLabel sets or clears a bookmark label on a session entry.
	// Empty label clears the label.
	AppendLabel(targetID, label string) error

	// FullTree returns every entry in the session for full DAG rendering.
	FullTree() []TreeNode
}

// ModelOverride holds API-sourced values that replace curated defaults.
type ModelOverride struct {
	Name          string
	ContextWindow int
	MaxTokens     int
}

// ModelManager provides model operations to commands and extensions.
// Implemented by the wiring layer which imports provider/ and config/.
type ModelManager interface {
	// Available returns all registered models.
	Available() []core.Model

	// Switch activates a model by its "provider/id" key.
	// Returns the model and a configured streaming provider.
	// The implementation handles provider creation and auth.
	Switch(id string) (core.Model, core.StreamProvider, error)

	// Sync updates the model catalog from an external source (e.g. models.dev).
	// Returns the number of models updated.
	Sync() (updated int, err error)

	// WriteWithOverrides regenerates models.yaml from the embedded curated
	// list with the given API overrides, writes it to disk, and reloads.
	WriteWithOverrides(overrides map[string]ModelOverride) (int, error)
}
