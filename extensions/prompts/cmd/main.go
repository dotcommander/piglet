// Prompts extension. Scans prompt template directories for .md files and
// registers each as a slash command with positional argument expansion.
package main

import (
	"github.com/dotcommander/piglet/extensions/prompts"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("prompts", "0.1.0")
	prompts.Register(e)
	e.Run()
}
