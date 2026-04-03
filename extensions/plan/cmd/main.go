package main

import (
	"github.com/dotcommander/piglet/extensions/plan"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("plan", "0.1.0")
	plan.Register(e)
	e.Run()
}
