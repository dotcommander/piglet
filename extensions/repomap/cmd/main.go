package main

import (
	"github.com/dotcommander/piglet/extensions/repomap"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("repomap", "0.1.0")
	repomap.Register(e)
	e.Run()
}
