// Safeguard extension binary. Blocks dangerous commands via interceptor.
// Supports profiles (strict/balanced/off), audit logging, and workspace scoping.
// Also provides a per-tool circuit breaker that disables flaky tools after
// consecutive failures. Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"time"

	"github.com/dotcommander/piglet/extensions/safeguard"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("safeguard", "0.3.0")
	safeguard.Register(e)
	// Per-tool circuit breaker: disable tools after 5 consecutive errors, re-enable after 30s.
	safeguard.RegisterBreaker(e, 5, 30*time.Second)
	e.Run()
}
