// Git context extension. Injects uncommitted changes, recent commits, and
// small diffs into the system prompt so the model knows the repo state.
package main

import (
	"github.com/dotcommander/piglet/extensions/gitcontext"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("gitcontext", "0.1.0")
	gitcontext.Register(e)
	e.Run()
}
