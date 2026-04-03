// Scaffold extension. Creates a new extension skeleton in the extensions directory.
package main

import (
	"github.com/dotcommander/piglet/extensions/scaffold"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("scaffold", "0.1.0")
	scaffold.Register(e)
	e.Run()
}
