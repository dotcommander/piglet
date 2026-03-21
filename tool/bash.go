package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"strings"
	"time"
)

func bashTool(app *ext.App) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "bash",
			Description: "Execute a shell command. Returns stdout, stderr, and exit code.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "Shell command to execute"},
					"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 30)"},
				},
				"required": []string{"command"},
			},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			command, _ := args["command"].(string)
			if command == "" {
				return textResult("error: command is required"), nil
			}

			timeout := intArg(args, "timeout", 30)
			if timeout < 1 {
				timeout = 30
			}
			if timeout > 300 {
				timeout = 300
			}

			cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()

			cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
			cmd.Dir = app.CWD()

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			var b strings.Builder
			if stdout.Len() > 0 {
				out := stdout.String()
				if len(out) > 100_000 {
					out = out[:100_000] + "\n... (output truncated)"
				}
				b.WriteString(out)
			}
			if stderr.Len() > 0 {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				errOut := stderr.String()
				if len(errOut) > 50_000 {
					errOut = errOut[:50_000] + "\n... (stderr truncated)"
				}
				b.WriteString("STDERR:\n")
				b.WriteString(errOut)
			}

			if err != nil {
				if cmdCtx.Err() == context.DeadlineExceeded {
					b.WriteString(fmt.Sprintf("\n\ncommand timed out after %ds", timeout))
				} else if exitErr, ok := err.(*exec.ExitError); ok {
					b.WriteString(fmt.Sprintf("\n\nexit code: %d", exitErr.ExitCode()))
				} else {
					b.WriteString(fmt.Sprintf("\n\nerror: %v", err))
				}
			}

			if b.Len() == 0 {
				b.WriteString("(no output)")
			}

			return textResult(b.String()), nil
		},
		PromptHint:   "Execute shell commands",
		PromptGuides: []string{"Always specify a reasonable timeout", "Avoid long-running commands"},
	}
}
