// Package sessioncmd registers the session-manipulation slash commands.
// Extracted from the compiled-in command/ package; see .work/specs/T5b-session-commands.md.
package sessioncmd

import (
	"github.com/dotcommander/piglet/sdk"
)

// Register wires /session, /search, /fork, /branch, /tree, /title and the
// ctrl+s shortcut. All commands use only the SDK; no ext.App access.
func Register(e *sdk.Extension) {
	registerSession(e)
	registerSearch(e)
	registerFork(e)
	registerBranch(e)
	registerTree(e)
	registerTitle(e)
	registerSessionShortcut(e)
}
