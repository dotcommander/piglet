package main

import (
	"github.com/dotcommander/piglet/extensions/lsp"
	"github.com/dotcommander/piglet/extensions/packs/internal/safety"
	"github.com/dotcommander/piglet/extensions/plan"
	"github.com/dotcommander/piglet/extensions/repomap"
	"github.com/dotcommander/piglet/extensions/sift"
	"github.com/dotcommander/piglet/extensions/suggest"
	"github.com/dotcommander/piglet/extensions/toolsearch"
	"github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("pack-code", "0.1.0")
	safety.Register(e, "lsp", lsp.Register)
	safety.Register(e, "repomap", repomap.Register)
	safety.Register(e, "sift", sift.Register)
	safety.Register(e, "plan", plan.Register)
	safety.Register(e, "suggest", suggest.Register)
	safety.Register(e, "toolsearch", toolsearch.Register)
	e.Run()
}
