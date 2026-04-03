// Package rtk provides the RTK bash-rewrite extension for piglet.
// RTK rewrites bash commands to reduce token output.
package rtk

import (
	_ "embed"

	"context"
	"maps"
	"os/exec"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/prompt.md
var defaultPrompt string

// Register adds rtk's interceptor and prompt section to the extension.
// If the rtk binary is not found in PATH, registration is skipped entirely.
func Register(e *sdk.Extension) {
	rtkPath, err := exec.LookPath("rtk")
	if err != nil {
		// RTK not found — skip all registrations
		return
	}

	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "rtk",
		Priority: 100,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
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
		},
	})

	e.RegisterPromptSection(sdk.PromptSectionDef{
		Title:   "RTK Token Optimization",
		Content: xdg.LoadOrCreateExt("rtk", "prompt.md", strings.TrimSpace(defaultPrompt)),
		Order:   90,
	})
}

func rewrite(ctx context.Context, rtkPath, command string) (string, error) {
	cmd := exec.CommandContext(ctx, rtkPath, "rewrite", command)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
