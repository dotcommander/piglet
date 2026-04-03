package safeguard

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// Register adds safeguard's interceptor to the extension.
// If the configured profile is "off", the interceptor is still registered but
// its Before hook is a no-op (allow-all). This keeps the pack entry point simple.
func Register(e *sdk.Extension) {
	cfg := LoadConfig()
	compiled := CompilePatterns(cfg.Patterns)
	audit := NewAuditLogger()

	// blocker is set in OnInit (when CWD is available) and read atomically in Before.
	// Before OnInit completes, calls fall through to allow (safe default).
	var blocker atomic.Pointer[func(context.Context, string, map[string]any) (bool, map[string]any, error)]

	if cfg.Profile != ProfileOff {
		e.OnInitAppend(func(e *sdk.Extension) {
			start := time.Now()
			e.Log("debug", "[safeguard] OnInit start")
			fn := BlockerWithConfig(cfg, compiled, e.CWD(), audit)
			blocker.Store(&fn)
			e.Log("debug", fmt.Sprintf("[safeguard] OnInit complete (%s)", time.Since(start)))
		})
	}

	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "safeguard",
		Priority: 2000,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			if fn := blocker.Load(); fn != nil {
				return (*fn)(ctx, toolName, args)
			}
			return true, args, nil
		},
	})
}
