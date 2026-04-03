// Cache extension binary. Provides persistent file-backed caching for other extensions.
// Library-only — no tools or commands registered.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/cache"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("cache", "0.1.0")
	cache.Register(e)
	e.Run()
}
