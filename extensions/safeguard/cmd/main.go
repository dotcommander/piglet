// Safeguard extension binary. Blocks dangerous commands via interceptor.
// Supports profiles (strict/balanced/off), audit logging, and workspace scoping.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/safeguard"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("safeguard", "0.2.0")
	safeguard.Register(e)
	e.Run()
}
