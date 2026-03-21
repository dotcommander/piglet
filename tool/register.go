// Package tool implements the 7 built-in tools for piglet.
// All tools register through the ext API, same as external extensions.
package tool

import "github.com/dotcommander/piglet/ext"

// RegisterBuiltins registers all built-in tools via the extension API.
func RegisterBuiltins(app *ext.App) {
	app.RegisterTool(readTool(app))
	app.RegisterTool(writeTool(app))
	app.RegisterTool(editTool(app))
	app.RegisterTool(bashTool(app))
	app.RegisterTool(grepTool(app))
	app.RegisterTool(findTool(app))
	app.RegisterTool(lsTool(app))
}
