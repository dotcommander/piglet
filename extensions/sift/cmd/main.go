package main

import (
	"github.com/dotcommander/piglet/extensions/sift"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("sift", "0.1.0")
	sift.Register(e)
	e.Run()
}
