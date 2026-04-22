package main

import (
	"github.com/dotcommander/piglet/extensions/admin"
	"github.com/dotcommander/piglet/extensions/background"
	"github.com/dotcommander/piglet/extensions/cmdcore"
	"github.com/dotcommander/piglet/extensions/export"
	extlist "github.com/dotcommander/piglet/extensions/extensions-list"
	"github.com/dotcommander/piglet/extensions/packs/internal/safety"
	"github.com/dotcommander/piglet/extensions/scaffold"
	"github.com/dotcommander/piglet/extensions/selfknowledge"
	"github.com/dotcommander/piglet/extensions/sessioncmd"
	"github.com/dotcommander/piglet/extensions/undo"
	"github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("pack-core", "0.1.0")
	safety.Register(e, "admin", admin.Register)
	cmdcore.Register(e)
	safety.Register(e, "export", export.Register)
	safety.Register(e, "extensions-list", extlist.Register)
	safety.Register(e, "undo", undo.Register)
	safety.Register(e, "scaffold", scaffold.Register)
	safety.Register(e, "background", background.Register)
	safety.Register(e, "sessioncmd", sessioncmd.Register)
	selfknowledge.Register(e)
	e.Run()
}
