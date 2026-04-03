package main

import (
	"github.com/dotcommander/piglet/extensions/inbox"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("inbox", "0.1.0")
	inbox.Register(e)
	e.Run()
}
