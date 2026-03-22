// Safeguard extension binary. Blocks dangerous bash commands via interceptor.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"

	"github.com/dotcommander/piglet/safeguard"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

func main() {
	compiled := safeguard.CompilePatterns(safeguard.LoadPatterns())
	if len(compiled) == 0 {
		// No patterns — still run so the host doesn't error, just no-op
		ext := sdk.New("safeguard", "0.1.0")
		ext.Run()
		return
	}

	blocker := safeguard.Blocker(compiled)

	ext := sdk.New("safeguard", "0.1.0")
	ext.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "safeguard",
		Priority: 2000,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			return blocker(ctx, toolName, args)
		},
	})
	ext.Run()
}
