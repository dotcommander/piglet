// Package subagent provides the tmux-based agent dispatch tool for piglet.
// Agents are spawned as full piglet instances in tmux panes, giving the user
// full visibility and intervention capability.
package subagent

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dotcommander/piglet/extensions/internal/safeexec"
	sdk "github.com/dotcommander/piglet/sdk"
)

const (
	defaultAbsoluteTimeout   = 30 * time.Minute
	defaultInactivityTimeout = 5 * time.Minute
	pollInterval             = 500 * time.Millisecond
)

// tmuxExtraEnv lists env vars beyond safeexec.DefaultEnvAllowlist that the
// tmux client process needs to locate its running server session.
// TMUX holds the socket path + session ID set by tmux itself; without it the
// split-window command cannot find the current session.
// TMUX_TMPDIR overrides the socket directory on some systems.
// *_API_KEY vars are forwarded so auth.GetAPIKey env-fallback works in the
// child piglet when the user has not stored keys in auth.json.
var tmuxExtraEnv = collectTmuxExtra()

func collectTmuxExtra() []string {
	var extra []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if key == "TMUX" || key == "TMUX_TMPDIR" || strings.HasSuffix(key, "_API_KEY") {
			extra = append(extra, kv)
		}
	}
	return extra
}

// Register adds the dispatch tool to the extension.
func Register(e *sdk.Extension) {
	// cache is captured by the Execute closure so repeated identical tasks
	// return the last result instead of re-spawning a tmux agent.
	cache := &dedupCache{}
	e.RegisterTool(sdk.ToolDef{
		Name:        "dispatch",
		Description: "Spawn a piglet agent in a tmux pane to handle a task independently. The agent runs as a full piglet instance with complete tool access and streaming visibility. The user can observe and intervene via the tmux pane. Results are returned when the agent completes.",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":  map[string]any{"type": "string", "description": "Task instructions for the agent"},
				"model": map[string]any{"type": "string", "description": "Model override (e.g. --model anthropic/claude-haiku-4-5)"},
				"split": map[string]any{"type": "string", "enum": []any{"horizontal", "vertical", "window"}, "description": "Tmux layout: horizontal split (default), vertical split, or new window"},
				"absolute_timeout_ms": map[string]any{
					"type":        "integer",
					"description": "Hard wall-clock timeout in milliseconds (default 1800000 = 30m). Set <= 0 to disable.",
				},
				"inactivity_timeout_ms": map[string]any{
					"type":        "integer",
					"description": "Kill the agent if its tmux pane shows no output for this many milliseconds (default 300000 = 5m). Catches stalled agents that connect but freeze mid-task. Set <= 0 to disable.",
				},
			},
			"required": []any{"task"},
		},
		PromptHint: "Spawn an independent agent in a tmux pane for focused research, analysis, or parallel work",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			task, _ := args["task"].(string)
			if task == "" {
				return sdk.ErrorResult("task is required"), nil
			}

			// Dedup recent-task: return cached result if the same normalized prompt
			// completed within recentTaskTTL. Exact/near-exact repeats only — prompts
			// that differ materially will re-run as normal.
			dedupKey := normalizePrompt(task)
			if cached, ok := cache.lookup(dedupKey); ok {
				return sdk.TextResult(cached), nil
			}

			// Verify tmux is available and we're inside a session
			if os.Getenv("TMUX") == "" {
				return sdk.ErrorResult("dispatch requires tmux — run piglet inside a tmux session"), nil
			}
			if _, err := exec.LookPath("tmux"); err != nil {
				return sdk.ErrorResult("tmux not found in PATH"), nil
			}

			// Parse timeout overrides — <= 0 means disabled.
			absolute := durationFromMs(args, "absolute_timeout_ms", defaultAbsoluteTimeout)
			inactivity := durationFromMs(args, "inactivity_timeout_ms", defaultInactivityTimeout)

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

			// Determine tmux split mode. -P -F "#{pane_id}" causes split-window /
			// new-window to print the new pane's id so we can query its activity.
			split, _ := args["split"].(string)
			var tmuxArgs []string
			switch split {
			case "vertical":
				tmuxArgs = []string{"split-window", "-v", "-P", "-F", "#{pane_id}", shellCmd}
			case "window":
				tmuxArgs = []string{"new-window", "-n", "agent-" + agentID, "-P", "-F", "#{pane_id}", shellCmd}
			default: // horizontal (default)
				tmuxArgs = []string{"split-window", "-h", "-P", "-F", "#{pane_id}", shellCmd}
			}

			// Spawn the tmux pane with a filtered environment. safeexec.FilterEnv
			// strips secrets (DB passwords, bearer tokens, etc.) not in its
			// allowlist. We append TMUX/TMUX_TMPDIR so the client can locate its
			// server session, and *_API_KEY vars so auth.GetAPIKey env-fallback
			// works in the child piglet process.
			var spawnOut, spawnErr bytes.Buffer
			cmd := exec.CommandContext(ctx, "tmux", tmuxArgs...)
			cmd.Env = append(safeexec.FilterEnv(nil), tmuxExtraEnv...)
			cmd.Stdout = &spawnOut
			cmd.Stderr = &spawnErr
			if err := cmd.Run(); err != nil {
				_ = os.RemoveAll(tmpDir)
				return sdk.ToolErr(sdk.ToolErrInternal,
					fmt.Sprintf("tmux spawn failed: %v", err),
					strings.TrimSpace(spawnErr.String())), nil
			}
			paneID := strings.TrimSpace(spawnOut.String())
			if paneID == "" {
				_ = os.RemoveAll(tmpDir)
				return sdk.ToolErr(sdk.ToolErrInternal,
					"tmux did not return pane id",
					"expected -P -F '#{pane_id}' to print a non-empty id"), nil
			}

			// Poll for result file (agent writes it on completion).
			// Two independent deadlines: absolute wall-clock and inactivity.
			spawnedAt := time.Now()
			timer := time.NewTimer(pollInterval)
			defer timer.Stop()

			for {
				select {
				case <-ctx.Done():
					return sdk.ToolErr("CANCELLED", "dispatch cancelled", ""), nil
				case <-timer.C:
				}

				// Result ready? Normal completion path — store for dedup then return.
				if data, err := os.ReadFile(resultPath); err == nil {
					_ = os.RemoveAll(tmpDir)
					result := strings.TrimSpace(string(data))
					var output string
					if result == "" {
						output = fmt.Sprintf("[agent %s completed with no output]", agentID)
					} else {
						output = fmt.Sprintf("[agent %s]\n\n%s", agentID, result)
					}
					cache.store(dedupKey, output)
					return sdk.TextResult(output), nil
				}

				// Absolute deadline.
				if absolute > 0 && time.Since(spawnedAt) > absolute {
					killPane(ctx, paneID)
					_ = os.RemoveAll(tmpDir)
					return sdk.ToolErr(sdk.ToolErrTimeout,
						fmt.Sprintf("agent %s exceeded absolute timeout %s (pane %s killed)",
							agentID, absolute, paneID),
						"increase absolute_timeout_ms or narrow the task"), nil
				}

				// Inactivity deadline.
				if inactivity > 0 {
					last, err := queryPaneActivity(ctx, paneID)
					if err == nil && paneStalled(time.Now(), last, inactivity) {
						killPane(ctx, paneID)
						_ = os.RemoveAll(tmpDir)
						idle := time.Since(last).Round(time.Second)
						return sdk.ToolErr(sdk.ToolErrTimeout,
							fmt.Sprintf("agent %s idle for %s (pane %s killed, inactivity limit %s)",
								agentID, idle, paneID, inactivity),
							"agent produced no output — may be stalled on network or blocked on input"), nil
					}
					// err != nil → pane_last_activity query failed; don't trip on transient
					// tmux-query errors. Fall through to next poll.
				}

				timer.Reset(pollInterval)
			}
		},
	})
}

// queryPaneActivity runs `tmux display-message -p -t <paneID> "#{pane_last_activity}"`
// and parses the unix-timestamp result.
// Returns a zero time.Time and non-nil error if the tmux call fails or the
// output is non-numeric.
func queryPaneActivity(ctx context.Context, paneID string) (time.Time, error) {
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", paneID, "#{pane_last_activity}").Output()
	if err != nil {
		return time.Time{}, err
	}
	s := strings.TrimSpace(string(out))
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse pane_last_activity %q: %w", s, err)
	}
	if sec == 0 {
		return time.Time{}, nil
	}
	return time.Unix(sec, 0), nil
}

// paneStalled reports whether lastActivity is older than limit relative to now.
// A zero lastActivity (tmux reports 0 for panes with no output yet) is never
// considered stalled — we wait for at least one output event before judging.
func paneStalled(now, lastActivity time.Time, limit time.Duration) bool {
	return !lastActivity.IsZero() && now.Sub(lastActivity) > limit
}

// killPane runs `tmux kill-pane -t <paneID>` and returns. Errors are ignored —
// the pane may have exited on its own between the stall check and the kill.
func killPane(ctx context.Context, paneID string) {
	_ = exec.CommandContext(ctx, "tmux", "kill-pane", "-t", paneID).Run()
}

// durationFromMs reads an integer milliseconds field from the tool args map.
// Missing, wrong-type, or NaN → fallback.
// <= 0 → returns 0 (disabled sentinel — caller branches on this).
// > 0 → returns the duration.
func durationFromMs(args map[string]any, key string, fallback time.Duration) time.Duration {
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	var ms int64
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) {
			return fallback
		}
		ms = int64(n)
	case int:
		ms = int64(n)
	case int64:
		ms = n
	default:
		return fallback
	}
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
