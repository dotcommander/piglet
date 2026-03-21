// Package rtk integrates the RTK (Rust Token Killer) CLI proxy as a piglet
// extension. When rtk is in PATH, it registers an interceptor that rewrites
// bash tool commands through `rtk rewrite` for 60-90% token savings.
//
// Config: set `rtk: false` to disable, omit for auto-detect.
package rtk

import (
	"context"
	"maps"
	"os/exec"
	"strings"

	"github.com/dotcommander/piglet/ext"
)

// Register adds the RTK interceptor and prompt hint to app.
// enabled: nil = auto-detect, true = require, false = skip.
func Register(app *ext.App, enabled *bool) {
	if enabled != nil && !*enabled {
		return
	}

	rtkPath, err := exec.LookPath("rtk")
	if err != nil {
		return
	}

	app.RegisterInterceptor(ext.Interceptor{
		Name:     "rtk",
		Priority: 100,
		Before:   rewriter(rtkPath),
	})

	app.RegisterPromptSection(ext.PromptSection{
		Title:   "RTK Token Optimization",
		Content: "Bash commands are automatically optimized by RTK for reduced token output. No action needed.",
		Order:   90, // low priority, informational
	})
}

func rewriter(rtkPath string) func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
	return func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
		if toolName != "bash" {
			return true, args, nil
		}

		command, _ := args["command"].(string)
		if command == "" {
			return true, args, nil
		}

		rewritten, err := rewrite(ctx, rtkPath, command)
		if err != nil || rewritten == "" || rewritten == command {
			return true, args, nil
		}

		modified := maps.Clone(args)
		modified["command"] = rewritten
		return true, modified, nil
	}
}

// rewrite calls `rtk rewrite <cmd>`. Returns empty on exit 1 (no rewrite).
func rewrite(ctx context.Context, rtkPath, command string) (string, error) {
	cmd := exec.CommandContext(ctx, rtkPath, "rewrite", command)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
