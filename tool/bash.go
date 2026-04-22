package tool

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/errfmt"
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
		InterruptBehavior: ext.InterruptBlock,
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			command, _ := args["command"].(string)
			if command == "" {
				return errfmt.ToolErr(errfmt.ToolErrInvalidArgs, "command is required", "provide a shell command to execute"), nil
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
			configureSysProcAttr(cmd)
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
				outputBody := b.String()
				switch {
				case cmdCtx.Err() == context.DeadlineExceeded:
					return toolBashErr(errfmt.ToolErrTimeout,
						fmt.Sprintf("command timed out after %ds", timeout),
						"increase timeout or run a narrower command",
						outputBody), nil
				case isExitError(err):
					exitErr := err.(*exec.ExitError)
					code := exitErr.ExitCode()
					// Some commands use non-zero exit to signal a non-error condition
					// (grep: no matches, diff: files differ, test: false). Classify so
					// the LLM doesn't misread these as failures and retry pointlessly.
					if label, isErr := classifyExitCode(command, code); !isErr {
						var sb strings.Builder
						sb.WriteString(outputBody)
						if sb.Len() > 0 {
							sb.WriteString("\n")
						}
						fmt.Fprintf(&sb, "(exit %d: %s)", code, label)
						return textResult(sb.String()), nil
					}
					return toolBashErr(errfmt.ToolErrExitNonzero,
						fmt.Sprintf("exit code %d", code),
						"inspect stderr — non-zero exit may be expected (e.g., grep with no matches)",
						outputBody), nil
				default:
					return toolBashErr(errfmt.ToolErrIO,
						fmt.Sprintf("command failed: %v", err),
						"",
						outputBody), nil
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

// classifyExitCode returns a human-readable label and error flag for a command's
// exit code. Some Unix commands use non-zero exits to signal non-error conditions
// (grep: no matches, diff: files differ, test: false). Classifying these lets the
// LLM interpret the result correctly instead of assuming failure.
//
// Returns (label, isError) where isError=false means treat as success with the
// label as an informational annotation. Returns ("", true) for any unclassified
// non-zero exit — caller should surface as an error (existing behavior).
func classifyExitCode(command string, code int) (string, bool) {
	if code != 1 {
		return "", true
	}
	for _, tok := range strings.Fields(command) {
		// Skip common prefix words that don't change the command identity.
		if tok == "sudo" || tok == "time" || tok == "env" {
			continue
		}
		// Skip VAR=value env assignments (but not paths containing '=').
		if strings.Contains(tok, "=") && !strings.ContainsAny(tok, "/\\") {
			continue
		}
		name := strings.ToLower(filepath.Base(tok))
		switch name {
		case "grep", "egrep", "fgrep", "rg":
			return "no matches", false
		case "diff", "cmp":
			return "files differ", false
		case "test", "[":
			return "condition false", false
		}
		// First recognizable token wasn't in the table — don't scan args.
		return "", true
	}
	return "", true
}
