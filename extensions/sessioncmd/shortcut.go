package sessioncmd

import (
	"context"

	"github.com/dotcommander/piglet/sdk"
)

func registerSessionShortcut(e *sdk.Extension) {
	e.RegisterShortcut(sdk.ShortcutDef{
		Key:         "ctrl+s",
		Description: "Open session picker",
		Handler: func(ctx context.Context) (*sdk.Action, error) {
			// Run the session picker directly (equivalent to `/session` with no args).
			_ = openSessionPicker(ctx, e)
			return nil, nil
		},
	})
}
