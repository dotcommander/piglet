// RTK extension binary. Rewrites bash commands through RTK for token savings.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/rtk"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("rtk", "0.1.0")
	rtk.Register(e)
	e.Run()
}
