// Pipeline extension binary. Runs multi-step workflows defined in YAML.
// Steps run sequentially with shared parameters, output passing, retries, and loops.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"github.com/dotcommander/piglet/extensions/pipeline"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("pipeline", "0.1.0")
	pipeline.Register(e)
	e.Run()
}
