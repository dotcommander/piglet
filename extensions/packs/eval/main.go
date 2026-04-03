package main

import (
	"github.com/dotcommander/piglet/extensions/eval"
	"github.com/dotcommander/piglet/extensions/packs/internal/safety"
	"github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("pack-eval", "0.1.0")
	safety.Register(e, "eval", eval.Register)
	e.Run()
}
