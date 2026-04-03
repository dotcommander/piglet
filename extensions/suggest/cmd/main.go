package main

import (
	"github.com/dotcommander/piglet/extensions/suggest"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("suggest", "0.1.0")
	suggest.Register(e)
	e.Run()
}
