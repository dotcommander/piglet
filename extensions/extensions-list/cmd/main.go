// Extensions-list extension. Registers /extensions command.
package main

import (
	extlist "github.com/dotcommander/piglet/extensions/extensions-list"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("extensions-list", "0.1.0")
	extlist.Register(e)
	e.Run()
}
