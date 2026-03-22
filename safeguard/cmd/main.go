// Safeguard extension binary. Blocks dangerous commands via interceptor.
// Supports profiles (strict/balanced/off), audit logging, and workspace scoping.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"sync/atomic"

	"github.com/dotcommander/piglet/safeguard"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

func main() {
	cfg := safeguard.LoadConfig()

	ext := sdk.New("safeguard", "0.2.0")

	if cfg.Profile == safeguard.ProfileOff {
		ext.Run()
		return
	}

	compiled := safeguard.CompilePatterns(cfg.Patterns)
	audit := safeguard.NewAuditLogger()

	// blocker is created in OnInit (when cwd is available) and stored atomically.
	// Before OnInit completes, Before calls fall through to allow (safe default).
	var blocker atomic.Pointer[func(context.Context, string, map[string]any) (bool, map[string]any, error)]

	ext.OnInit(func(e *sdk.Extension) {
		fn := safeguard.BlockerWithConfig(cfg, compiled, e.CWD(), audit)
		blocker.Store(&fn)
	})

	ext.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "safeguard",
		Priority: 2000,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			if fn := blocker.Load(); fn != nil {
				return (*fn)(ctx, toolName, args)
			}
			return true, args, nil
		},
	})

	ext.Run()
}
