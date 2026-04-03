// Bulk extension binary. Runs a shell command across many items in parallel.
// Sources: git_repos, dirs, files, list. Filters: shell predicates or git conditions.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/bulk"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("bulk", "0.1.0")
	bulk.Register(e)
	e.Run()
}
