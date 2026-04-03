package main

import (
	"github.com/dotcommander/piglet/extensions/tokengate"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("tokengate", "0.1.0")
	tokengate.Register(e)
	e.Run()
}
