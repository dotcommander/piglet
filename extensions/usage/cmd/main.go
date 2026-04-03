// Usage extension binary. Tracks and displays token usage statistics.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/usage"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("usage", "0.1.0")
	usage.Register(e)
	e.Run()
}
