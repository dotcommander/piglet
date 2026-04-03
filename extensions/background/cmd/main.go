// Background extension. Registers /bg and /bg-cancel commands.
package main

import (
	"github.com/dotcommander/piglet/extensions/background"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("background", "0.1.0")
	background.Register(e)
	e.Run()
}
