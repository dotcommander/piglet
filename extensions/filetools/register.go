// Package filetools registers the grep, find, and ls tools as an external
// extension for piglet. It is bundled into pack-code; there is no standalone binary.
package filetools

import (
	sdk "github.com/dotcommander/piglet/sdk"
)

// Register wires the grep, find, and ls tools into the given SDK extension.
func Register(e *sdk.Extension) {
	registerGrep(e)
	registerFind(e)
	registerLs(e)
}
