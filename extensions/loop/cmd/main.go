// Loop extension binary. Runs a prompt or slash command on a recurring interval.
// Communicates with the piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/loop"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("loop", "0.1.0")
	loop.Register(e)
	e.Run()
}
