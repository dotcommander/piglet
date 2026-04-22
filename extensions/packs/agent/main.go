package main

import (
	"github.com/dotcommander/piglet/extensions/autotitle"
	"github.com/dotcommander/piglet/extensions/clipboard"
	"github.com/dotcommander/piglet/extensions/guardrail"
	"github.com/dotcommander/piglet/extensions/loop"
	"github.com/dotcommander/piglet/extensions/packs/internal/safety"
	"github.com/dotcommander/piglet/extensions/provider"
	"github.com/dotcommander/piglet/extensions/rtk"
	"github.com/dotcommander/piglet/extensions/safeguard"
	"github.com/dotcommander/piglet/extensions/subagent"
	"github.com/dotcommander/piglet/extensions/toolbreaker"
	"github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("pack-agent", "0.1.0")
	safety.Register(e, "safeguard", safeguard.Register)
	safety.Register(e, "guardrail", guardrail.Register)
	safety.Register(e, "rtk", rtk.Register)
	safety.Register(e, "autotitle", autotitle.Register)
	safety.Register(e, "clipboard", clipboard.Register)
	safety.Register(e, "subagent", subagent.Register)
	safety.Register(e, "provider", provider.Register)
	safety.Register(e, "loop", loop.Register)
	safety.Register(e, "toolbreaker", toolbreaker.Register)
	e.Run()
}
