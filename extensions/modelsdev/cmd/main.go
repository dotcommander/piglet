// Modelsdev extension binary. Syncs model metadata from models.dev on init.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/modelsdev"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("modelsdev", "0.1.0")
	modelsdev.Register(e)
	e.Run()
}
