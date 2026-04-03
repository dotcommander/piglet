package cache

import "github.com/dotcommander/piglet/sdk"

// Register registers the cache extension. Cache is a library-only extension —
// no tools, commands, or prompt sections are registered.
func Register(_ *sdk.Extension) {}
