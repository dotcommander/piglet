package compact

import (
	_ "embed"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/compact-system.md
var defaultCompactSystem string

// Storer is the narrow store interface required by compaction.
// *memory.Store satisfies this interface.
type Storer interface {
	List(category string) []Fact
	Set(key, value, category string) error
	Get(key string) (Fact, bool)
	Clear() error
}

// Fact is a minimal key/value memory entry as seen by compaction.
// Mirrors the fields of extensions/memory.Fact that compaction uses.
type Fact struct {
	Key       string
	Value     string
	Category  string
	Relations []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReinjectFunc returns a post-compact re-injection message for context
// items that should survive compaction. An empty return means nothing
// to reinject. This callback is provided by the memory package to avoid
// an import cycle (compact/ must not import memory/).
type ReinjectFunc func(s Storer) string

// Register wires the rolling-memory compactor onto the extension.
// reinject is a callback that builds a post-compact context re-injection
// message from the store; pass memory.MakeReinjectFunc() from the caller.
func Register(x *sdk.Extension, s Storer, reinject ReinjectFunc) {
	x.RegisterCompactor(sdk.CompactorDef{
		Name:      "rolling-memory",
		Threshold: 50000,
		Compact:   newHandler(x, s, reinject).Handle,
	})
}
