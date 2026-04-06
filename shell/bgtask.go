package shell

import (
	"strings"

	"github.com/dotcommander/piglet/core"
)

// bgEntry tracks a single named background task.
type bgEntry struct {
	agent   *core.Agent
	eventCh <-chan core.Event
	prompt  string
	result  strings.Builder
}
