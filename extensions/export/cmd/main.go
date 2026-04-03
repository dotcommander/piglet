// Export extension. Exports the current conversation to a markdown file.
package main

import (
	"github.com/dotcommander/piglet/extensions/export"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("export", "0.1.0")
	export.Register(e)
	e.Run()
}
