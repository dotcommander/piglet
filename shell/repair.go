package shell

import (
	"time"

	"github.com/dotcommander/piglet/core"
)

// repairMessageSequence scans the conversation for orphaned tool calls —
// ToolCall entries in an AssistantMessage that have no corresponding
// ToolResultMessage — and synthesizes placeholder results so the LLM
// API never sees a dangling tool_use without a tool_result.
//
// Returns the repaired slice (may be the same slice if no repair needed).
func repairMessageSequence(msgs []core.Message) []core.Message {
	// Collect all tool call IDs that have results.
	hasResult := make(map[string]struct{})
	for _, m := range msgs {
		if tr, ok := m.(*core.ToolResultMessage); ok {
			hasResult[tr.ToolCallID] = struct{}{}
		}
	}

	// Walk assistant messages looking for orphaned tool calls.
	var repairs []*core.ToolResultMessage
	for _, m := range msgs {
		am, ok := m.(*core.AssistantMessage)
		if !ok {
			continue
		}
		for _, c := range am.Content {
			tc, ok := c.(core.ToolCall)
			if !ok {
				continue
			}
			if _, found := hasResult[tc.ID]; !found {
				repairs = append(repairs, &core.ToolResultMessage{
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Content: []core.ContentBlock{
						core.TextContent{Text: "[tool call interrupted — no result available]"},
					},
					IsError:   true,
					Timestamp: time.Now(),
				})
			}
		}
	}

	if len(repairs) == 0 {
		return msgs
	}

	// Append synthesized results at the end before re-sending.
	out := make([]core.Message, len(msgs), len(msgs)+len(repairs))
	copy(out, msgs)
	for _, r := range repairs {
		out = append(out, r)
	}
	return out
}
