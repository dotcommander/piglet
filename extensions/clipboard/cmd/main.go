// Clipboard extension binary. Reads images from the system clipboard.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/clipboard"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("clipboard", "0.1.0")
	clipboard.Register(e)
	e.Run()
}
