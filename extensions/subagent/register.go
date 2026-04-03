// Package subagent provides the tmux-based agent dispatch tool for piglet.
// Agents are spawned as full piglet instances in tmux panes, giving the user
// full visibility and intervention capability.
package subagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	sdk "github.com/dotcommander/piglet/sdk"
)

// Register adds the dispatch tool to the extension.
func Register(e *sdk.Extension) {
	e.RegisterTool(sdk.ToolDef{
		Name:        "dispatch",
		Description: "Spawn a piglet agent in a tmux pane to handle a task independently. The agent runs as a full piglet instance with complete tool access and streaming visibility. The user can observe and intervene via the tmux pane. Results are returned when the agent completes.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":  map[string]any{"type": "string", "description": "Task instructions for the agent"},
				"model": map[string]any{"type": "string", "description": "Model override (e.g. --model anthropic/claude-haiku-4-5)"},
				"split": map[string]any{"type": "string", "enum": []any{"horizontal", "vertical", "window"}, "description": "Tmux layout: horizontal split (default), vertical split, or new window"},
			},
			"required": []any{"task"},
		},
		PromptHint: "Spawn an independent agent in a tmux pane for focused research, analysis, or parallel work",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			task, _ := args["task"].(string)
			if task == "" {
				return sdk.ErrorResult("task is required"), nil
			}

			// Verify tmux is available and we're inside a session
			if os.Getenv("TMUX") == "" {
				return sdk.ErrorResult("dispatch requires tmux — run piglet inside a tmux session"), nil
			}
			if _, err := exec.LookPath("tmux"); err != nil {
				return sdk.ErrorResult("tmux not found in PATH"), nil
			}

			// Create result directory
			agentID := uuid.New().String()[:8]
			tmpDir := filepath.Join(os.TempDir(), "piglet-agent-"+agentID)
			if err := os.MkdirAll(tmpDir, 0700); err != nil {
				return sdk.ErrorResult(fmt.Sprintf("create agent dir: %v", err)), nil
			}

			resultPath := filepath.Join(tmpDir, "result.md")

			// Build piglet command
			var cmdParts []string
			cmdParts = append(cmdParts, "piglet")
			if model, _ := args["model"].(string); model != "" {
				cmdParts = append(cmdParts, "--result", resultPath, "--model", model)
			} else {
				cmdParts = append(cmdParts, "--result", resultPath)
			}
			// Quote the task for shell safety
			cmdParts = append(cmdParts, fmt.Sprintf("%q", task))

			pigletCmd := strings.Join(cmdParts, " ")

			// Wrap: run piglet, then hold pane open briefly so user can see result
			shellCmd := fmt.Sprintf("%s; echo ''; echo '[agent %s complete — press enter to close]'; read", pigletCmd, agentID)

			// Determine tmux split mode
			split, _ := args["split"].(string)
			var tmuxArgs []string
			switch split {
			case "vertical":
				tmuxArgs = []string{"split-window", "-v", shellCmd}
			case "window":
				tmuxArgs = []string{"new-window", "-n", "agent-" + agentID, shellCmd}
			default: // horizontal (default)
				tmuxArgs = []string{"split-window", "-h", shellCmd}
			}

			// Spawn the tmux pane
			cmd := exec.CommandContext(ctx, "tmux", tmuxArgs...)
			if out, err := cmd.CombinedOutput(); err != nil {
				_ = os.RemoveAll(tmpDir)
				return sdk.ErrorResult(fmt.Sprintf("tmux spawn failed: %v: %s", err, string(out))), nil
			}

			// Poll for result file (agent writes it on completion)
			timeout := 5 * time.Minute
			deadline := time.Now().Add(timeout)
			pollInterval := 500 * time.Millisecond
			timer := time.NewTimer(pollInterval)
			defer timer.Stop()

			for {
				select {
				case <-ctx.Done():
					return sdk.ErrorResult("dispatch cancelled"), nil
				case <-timer.C:
				}

				if time.Now().After(deadline) {
					return sdk.TextResult(fmt.Sprintf("[agent %s timed out after %s — check tmux pane for status]", agentID, timeout)), nil
				}

				if data, err := os.ReadFile(resultPath); err == nil {
					// Clean up temp dir
					_ = os.RemoveAll(tmpDir)

					result := strings.TrimSpace(string(data))
					if result == "" {
						return sdk.TextResult(fmt.Sprintf("[agent %s completed with no output]", agentID)), nil
					}
					return sdk.TextResult(fmt.Sprintf("[agent %s]\n\n%s", agentID, result)), nil
				}

				timer.Reset(pollInterval)
			}
		},
	})
}
