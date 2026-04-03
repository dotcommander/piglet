package main

import (
	"github.com/dotcommander/piglet/extensions/changelog"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("changelog", "0.1.0")
	changelog.Register(e)
	e.Run()
}
