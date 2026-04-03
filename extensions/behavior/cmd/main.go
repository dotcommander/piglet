// Behavior extension. Loads behavioral guidelines from ~/.config/piglet/behavior.md
// and injects them as the earliest system prompt section.
package main

import (
	"github.com/dotcommander/piglet/extensions/behavior"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("behavior", "0.1.0")
	behavior.Register(e)
	e.Run()
}
