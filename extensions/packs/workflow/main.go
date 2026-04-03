package main

import (
	"github.com/dotcommander/piglet/extensions/bulk"
	"github.com/dotcommander/piglet/extensions/cache"
	"github.com/dotcommander/piglet/extensions/modelsdev"
	"github.com/dotcommander/piglet/extensions/packs/internal/safety"
	"github.com/dotcommander/piglet/extensions/pipeline"
	"github.com/dotcommander/piglet/extensions/usage"
	"github.com/dotcommander/piglet/extensions/webfetch"
	"github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("pack-workflow", "0.1.0")
	safety.Register(e, "pipeline", pipeline.Register)
	safety.Register(e, "bulk", bulk.Register)
	safety.Register(e, "webfetch", webfetch.Register)
	safety.Register(e, "cache", cache.Register)
	safety.Register(e, "usage", usage.Register)
	safety.Register(e, "modelsdev", modelsdev.Register)
	e.Run()
}
