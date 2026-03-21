package ext

import (
	"time"

	"github.com/dotcommander/piglet/core"
)

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

	// SetTitle updates the current session's title.
	SetTitle(title string) error
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
}
