// Session-tools extension. Registers /search, /branch, /title, /handoff commands,
// session_query and handoff tools, and handoff prompt section.
package main

import (
	sessiontools "github.com/dotcommander/piglet/extensions/session-tools"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("session-tools", "0.2.0")
	sessiontools.Register(e)
	e.Run()
}
