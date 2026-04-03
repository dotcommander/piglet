// Cron scheduling extension for piglet.
package main

import (
	"github.com/dotcommander/piglet/extensions/cron"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("cron", "0.1.0")
	cron.Register(e)
	e.Run()
}
