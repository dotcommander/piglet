// Memory extension binary. Persistent per-project key-value memory.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/memory"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("memory", "0.1.0")
	memory.Register(e)
	e.Run()
}
