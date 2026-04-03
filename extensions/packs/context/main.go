package main

import (
	"github.com/dotcommander/piglet/extensions/behavior"
	"github.com/dotcommander/piglet/extensions/distill"
	"github.com/dotcommander/piglet/extensions/gitcontext"
	"github.com/dotcommander/piglet/extensions/inbox"
	"github.com/dotcommander/piglet/extensions/memory"
	"github.com/dotcommander/piglet/extensions/packs/internal/safety"
	"github.com/dotcommander/piglet/extensions/prompts"
	"github.com/dotcommander/piglet/extensions/recall"
	"github.com/dotcommander/piglet/extensions/route"
	sessiontools "github.com/dotcommander/piglet/extensions/session-tools"
	"github.com/dotcommander/piglet/extensions/skill"
	"github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("pack-context", "0.1.0")
	safety.Register(e, "memory", memory.Register)
	safety.Register(e, "skill", skill.Register)
	safety.Register(e, "gitcontext", gitcontext.Register)
	safety.Register(e, "behavior", behavior.Register)
	safety.Register(e, "prompts", prompts.Register)
	safety.Register(e, "session-tools", sessiontools.Register)
	safety.Register(e, "inbox", inbox.Register)
	safety.Register(e, "distill", distill.Register)
	safety.Register(e, "recall", recall.Register)
	safety.Register(e, "route", route.Register)
	e.Run()
}
