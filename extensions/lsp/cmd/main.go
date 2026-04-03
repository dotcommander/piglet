package main

import (
	"github.com/dotcommander/piglet/extensions/lsp"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("lsp", "0.1.0")
	lsp.Register(e)
	e.Run()
}
