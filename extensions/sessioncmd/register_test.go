package sessioncmd

import (
	"testing"

	"github.com/dotcommander/piglet/sdk"
	"github.com/stretchr/testify/assert"
)

// TestRegisterNoPanic verifies that Register does not panic.
// sdk.RegisterCommand panics on empty Name or nil Handler, so this test
// catches any registration wiring mistakes.
func TestRegisterNoPanic(t *testing.T) {
	t.Parallel()

	e := sdk.New("sessioncmd-test", "0.1.0")
	assert.NotPanics(t, func() {
		Register(e)
	})
}
