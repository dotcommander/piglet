// Provider extension binary. Delegates LLM streaming to piglet provider implementations.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/provider"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("provider", "0.1.0")
	provider.Register(e)
	e.Run()
}
