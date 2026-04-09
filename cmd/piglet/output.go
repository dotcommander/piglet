package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/shell"
)

// toolLabel formats a tool call for display in print mode.
func toolLabel(e core.EventToolStart) string {
	summary := shell.ToolSummary(e.ToolName, e.Args)
	return "[tool: " + summary + "]"
}

// requireAgentStarted checks that a Submit response started the agent.
func requireAgentStarted(resp shell.Response) error {
	if resp.Kind != shell.ResponseAgentStarted {
		if resp.Error != nil {
			return resp.Error
		}
		return fmt.Errorf("unexpected response: %v", resp.Kind)
	}
	return nil
}

// extractAgentError checks if the assistant message indicates an error stop reason.
func extractAgentError(msg *core.AssistantMessage) error {
	if msg == nil {
		return nil
	}
	if msg.StopReason == core.StopReasonError || msg.StopReason == core.StopReasonAborted {
		errMsg := msg.Error
		if errMsg == "" {
			errMsg = string(msg.StopReason)
		}
		return fmt.Errorf("agent error: %s", errMsg)
	}
	return nil
}

// submitAndWait submits a prompt to the agent and processes the event stream,
// calling handle for each event. Returns nil on completion, ctx.Err() on cancel.
func submitAndWait(ctx context.Context, sh *shell.Shell, prompt string, handle func(core.Event)) error {
	resp := sh.Submit(prompt)
	if err := requireAgentStarted(resp); err != nil {
		return err
	}
	for {
		select {
		case evt, ok := <-resp.Events:
			if !ok {
				return nil
			}
			handle(evt)
			sh.ProcessEvent(evt)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func runPrint(ctx context.Context, sh *shell.Shell, userPrompt, resultPath string) error {
	var agentErr error
	var resultBuf strings.Builder

	loopErr := submitAndWait(ctx, sh, userPrompt, func(evt core.Event) {
		switch e := evt.(type) {
		case core.EventStreamDelta:
			if e.Kind == "text" {
				fmt.Print(e.Delta)
				if resultPath != "" {
					resultBuf.WriteString(e.Delta)
				}
			}
		case core.EventToolStart:
			fmt.Fprintf(os.Stderr, "\n%s\n", toolLabel(e))
		case core.EventToolEnd:
			if e.IsError {
				fmt.Fprintf(os.Stderr, "[tool error: %s]\n", e.ToolName)
			}
		case core.EventRetry:
			fmt.Fprintf(os.Stderr, "[retry %d/%d: %s]\n", e.Attempt, e.Max, e.Error)
		case core.EventCompact:
			fmt.Fprintf(os.Stderr, "[compacted: %d → %d messages at %d tokens]\n", e.Before, e.After, e.TokensAtCompact)
		case core.EventMaxTurns:
			fmt.Fprintf(os.Stderr, "[max turns reached: %d/%d]\n", e.Count, e.Max)
			agentErr = fmt.Errorf("agent stopped: max turns (%d) reached", e.Max)
		case core.EventAgentEnd:
			fmt.Println()
		}

		if e, ok := evt.(core.EventTurnEnd); ok {
			if e.Assistant != nil {
				u := e.Assistant.Usage
				slog.Debug("turn complete",
					"input_tokens", u.InputTokens,
					"output_tokens", u.OutputTokens,
					"cache_read_tokens", u.CacheReadTokens,
					"cache_write_tokens", u.CacheWriteTokens,
				)
				if err := extractAgentError(e.Assistant); err != nil {
					agentErr = err
				}
			}
		}

		drainNotifications(sh)
	})

	if loopErr != nil {
		return loopErr
	}
	if resultPath != "" {
		writeResultFile(resultPath, resultBuf.String())
	}
	return agentErr
}

// writeResultFile atomically writes the result to a file (temp + rename).
func writeResultFile(path, content string) {
	dir := filepath.Dir(path)
	tmp, tmpErr := os.CreateTemp(dir, ".piglet-result-*")
	if tmpErr != nil {
		slog.Warn("create temp result file", "path", path, "error", tmpErr)
		return
	}
	tmpName := tmp.Name()

	_, writeErr := tmp.WriteString(content)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpName)
		if writeErr != nil {
			slog.Warn("write result file", "path", path, "error", writeErr)
		} else {
			slog.Warn("close temp result file", "path", path, "error", closeErr)
		}
		return
	}

	if renameErr := os.Rename(tmpName, path); renameErr != nil {
		os.Remove(tmpName)
		// Cross-filesystem fallback
		if copyErr := os.WriteFile(path, []byte(content), 0600); copyErr != nil {
			slog.Warn("write result file", "path", path, "error", copyErr)
		}
	}
}

func runJSON(ctx context.Context, sh *shell.Shell, userPrompt string) error {
	enc := json.NewEncoder(os.Stdout)
	var agentErr error

	loopErr := submitAndWait(ctx, sh, userPrompt, func(evt core.Event) {
		switch e := evt.(type) {
		case core.EventStreamDelta:
			_ = enc.Encode(map[string]any{"type": "stream_delta", "kind": e.Kind, "index": e.Index, "delta": e.Delta})
		case core.EventToolStart:
			_ = enc.Encode(map[string]any{"type": "tool_start", "tool": e.ToolName, "args": e.Args})
		case core.EventToolEnd:
			_ = enc.Encode(map[string]any{"type": "tool_end", "tool": e.ToolName, "is_error": e.IsError})
		case core.EventRetry:
			_ = enc.Encode(map[string]any{"type": "retry", "attempt": e.Attempt, "max": e.Max, "error": e.Error})
		case core.EventCompact:
			_ = enc.Encode(map[string]any{"type": "compact", "before": e.Before, "after": e.After, "tokens": e.TokensAtCompact})
		case core.EventMaxTurns:
			_ = enc.Encode(map[string]any{"type": "max_turns", "count": e.Count, "max": e.Max})
			agentErr = fmt.Errorf("agent stopped: max turns (%d) reached", e.Max)
		case core.EventAgentEnd:
			_ = enc.Encode(map[string]any{"type": "agent_end"})
		}

		if e, ok := evt.(core.EventTurnEnd); ok {
			if e.Assistant != nil {
				a := e.Assistant
				u := a.Usage
				_ = enc.Encode(map[string]any{
					"type":        "turn_end",
					"model":       a.Model,
					"provider":    a.Provider,
					"stop_reason": string(a.StopReason),
					"usage": map[string]any{
						"input_tokens":       u.InputTokens,
						"output_tokens":      u.OutputTokens,
						"cache_read_tokens":  u.CacheReadTokens,
						"cache_write_tokens": u.CacheWriteTokens,
						"cost":               u.Cost,
					},
				})
				if err := extractAgentError(e.Assistant); err != nil {
					agentErr = err
					_ = enc.Encode(map[string]any{"type": "error", "message": err.Error()})
				}
			}
		}
	})

	if loopErr != nil {
		return loopErr
	}
	return agentErr
}
