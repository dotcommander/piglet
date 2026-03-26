package tool

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// BashConfig holds configurable limits for the bash tool. Zero values use defaults.
type BashConfig struct {
	DefaultTimeout int // seconds, default 30
	MaxTimeout     int // seconds, default 300
	MaxStdout      int // bytes, default 100000
	MaxStderr      int // bytes, default 50000
}

func (c BashConfig) withDefaults() BashConfig {
	if c.DefaultTimeout <= 0 {
		c.DefaultTimeout = 30
	}
	if c.MaxTimeout <= 0 {
		c.MaxTimeout = 300
	}
	if c.MaxStdout <= 0 {
		c.MaxStdout = 100_000
	}
	if c.MaxStderr <= 0 {
		c.MaxStderr = 50_000
	}
	return c
}

func bashTool(app *ext.App, cfg BashConfig) *ext.ToolDef {
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

			timeout := intArg(args, "timeout", cfg.DefaultTimeout)
			if timeout < 1 {
				timeout = cfg.DefaultTimeout
			}
			if timeout > cfg.MaxTimeout {
				timeout = cfg.MaxTimeout
			}

			cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()

			cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
			cmd.Dir = app.CWD()
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Cancel = func() error {
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			cmd.WaitDelay = 3 * time.Second

			stdout := &boundedWriter{limit: cfg.MaxStdout}
			stderr := &boundedWriter{limit: cfg.MaxStderr}
			cmd.Stdout = stdout
			cmd.Stderr = stderr

			err := cmd.Run()

			var b strings.Builder
			if stdout.Len() > 0 {
				b.WriteString(stdout.String())
				if stdout.Truncated() {
					b.WriteString("\n... (output truncated)")
				}
			}
			if stderr.Len() > 0 {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString("STDERR:\n")
				b.WriteString(stderr.String())
				if stderr.Truncated() {
					b.WriteString("\n... (stderr truncated)")
				}
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
