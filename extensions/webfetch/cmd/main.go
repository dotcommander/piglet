// Webfetch extension binary. Provides web_fetch and web_search tools.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/webfetch"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("webfetch", "0.2.0")
	webfetch.Register(e)
	e.Run()
}
