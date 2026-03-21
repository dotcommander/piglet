package prompt

import (
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

const selfKnowledgeOrder = 20 // before project docs (30), git context (40), memory (50)

// RegisterSelfKnowledge registers a prompt section that describes piglet's
// current capabilities: registered tools, commands, and shortcuts.
// Built dynamically from the live ext.App state so it stays accurate
// as extensions are loaded.
func RegisterSelfKnowledge(app *ext.App) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Working directory: %s\n", app.CWD()))
	b.WriteString(fmt.Sprintf("Platform: %s\n", runtime.GOOS))
	b.WriteString(fmt.Sprintf("Current time: %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST")))

	// Tools
	defs := app.ToolDefs()
	if len(defs) > 0 {
		b.WriteString("Registered tools: ")
		names := make([]string, len(defs))
		for i, d := range defs {
			names[i] = d.Name
		}
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("\n\n")
	}

	// Commands
	cmds := app.Commands()
	if len(cmds) > 0 {
		b.WriteString("Slash commands: /")
		b.WriteString(strings.Join(slices.Sorted(maps.Keys(cmds)), ", /"))
		b.WriteString("\n\n")
	}

	// Shortcuts
	shortcuts := app.Shortcuts()
	if len(shortcuts) > 0 {
		b.WriteString("Keyboard shortcuts:\n")
		for key, sc := range shortcuts {
			b.WriteString(fmt.Sprintf("- %s — %s\n", key, sc.Description))
		}
		b.WriteString("\n")
	}

	// Config paths
	if dir, err := config.ConfigDir(); err == nil {
		b.WriteString(fmt.Sprintf("Config directory: %s\n", dir))
	}

	content := strings.TrimSpace(b.String())
	if content == "" {
		return
	}

	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Current Capabilities",
		Content: content,
		Order:   selfKnowledgeOrder,
	})
}
