package modelsdev

import (
	"context"
	"fmt"
	"time"

	"github.com/dotcommander/piglet/sdk"
)

const refreshTimeout = 10 * time.Second

// Register registers the modelsdev extension's OnInit handler and commands.
func Register(e *sdk.Extension) {
	e.OnInit(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[modelsdev] OnInit start")

		if !CacheStale() {
			x.Log("debug", fmt.Sprintf("[modelsdev] OnInit complete — cache fresh (%s)", time.Since(start)))
			return
		}
		// Stale-while-revalidate: models.yaml has last-known-good data.
		// Refresh in background — never block the initialize handshake.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
			defer cancel()

			if _, err := Refresh(ctx, x); err != nil {
				x.Log("warn", "modelsdev: "+err.Error())
			}
		}()

		x.Log("debug", fmt.Sprintf("[modelsdev] OnInit complete — refresh running in background (%s)", time.Since(start)))
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "models-sync",
		Description: "Fetch latest model data from models.dev and regenerate models.yaml",
		Handler: func(ctx context.Context, _ string) error {
			e.ShowMessage("Fetching models from models.dev...")
			n, err := Refresh(ctx, e)
			if err != nil {
				e.ShowMessage("Sync failed: " + err.Error())
				return nil
			}
			e.ShowMessage(fmt.Sprintf("models.yaml regenerated — %d model(s) loaded", n))
			return nil
		},
	})
}
