// RTK extension binary. Rewrites bash commands through RTK for token savings.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"maps"
	"os/exec"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk/go"
)

func main() {
	rtkPath, err := exec.LookPath("rtk")
	if err != nil {
		// RTK not found — run as no-op extension
		ext := sdk.New("rtk", "0.1.0")
		ext.Run()
		return
	}

	ext := sdk.New("rtk", "0.1.0")

	ext.RegisterInterceptor(sdk.InterceptorDef{
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

	ext.RegisterPromptSection(sdk.PromptSectionDef{
		Title:   "RTK Token Optimization",
		Content: "Bash commands are automatically optimized by RTK for reduced token output. No action needed.",
		Order:   90,
	})

	ext.Run()
}

func rewrite(ctx context.Context, rtkPath, command string) (string, error) {
	cmd := exec.CommandContext(ctx, rtkPath, "rewrite", command)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
