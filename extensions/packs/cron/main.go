package main

import (
	"github.com/dotcommander/piglet/extensions/cron"
	"github.com/dotcommander/piglet/extensions/packs/internal/safety"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("pack-cron", "0.1.0")
	safety.Register(e, "cron", cron.Register)
	e.Run()
}
