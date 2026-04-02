// Package tool implements the 7 built-in tools for piglet.
// All tools register through the ext API, same as external extensions.
package tool

import (
	"time"

	"github.com/dotcommander/piglet/ext"
)

// ToolConfig holds configurable defaults for built-in tools.
type ToolConfig struct {
	ReadLimit int // max lines per read; 0 = default (2000)
	GrepLimit int // max grep matches; 0 = default (100)
}

func (c ToolConfig) readLimit() int {
	if c.ReadLimit > 0 {
		return c.ReadLimit
	}
	return 2000
}

func (c ToolConfig) grepLimit() int {
	if c.GrepLimit > 0 {
		return c.GrepLimit
	}
	return 100
}

// RegisterBuiltins registers all built-in tools via the extension API.
func RegisterBuiltins(app *ext.App, bash BashConfig, tools ToolConfig) {
	bash = bash.withDefaults()
	ft := &fileTracker{reads: make(map[string]time.Time)}
	app.RegisterTool(readTool(app, tools, ft))
	app.RegisterTool(writeTool(app, ft))
	app.RegisterTool(editTool(app, ft))
	app.RegisterTool(bashTool(app, bash))
	app.RegisterTool(grepTool(app, tools))
	app.RegisterTool(findTool(app))
	app.RegisterTool(lsTool(app))
}
