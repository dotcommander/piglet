// Package memory provides persistent per-project memory for piglet.
// All registration goes through the ext API — tools, commands, and prompt sections.
package memory

import "github.com/dotcommander/piglet/ext"

// Register creates a memory store for the current working directory and
// registers tools, the /memory command, and a prompt section via app.
// Errors are non-fatal: if the store can't be created, memory is silently skipped.
func Register(app *ext.App) {
	store, err := NewStore(app.CWD())
	if err != nil {
		return
	}

	registerTools(app, store)
	registerCommand(app, store)
	registerPromptSection(app, store)
}
