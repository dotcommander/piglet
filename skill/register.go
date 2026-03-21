package skill

import (
	"path/filepath"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

// Register sets up the skill extension: tools, command, prompt section, and trigger.
// Silently skipped if no skills directory exists or it's empty.
func Register(app *ext.App) {
	dir, err := config.ConfigDir()
	if err != nil {
		return
	}
	store := NewStore(filepath.Join(dir, "skills"))
	if len(store.List()) == 0 {
		return
	}

	registerTools(app, store)
	registerCommand(app, store)
	registerPromptSection(app, store)
	registerTrigger(app, store)
}
