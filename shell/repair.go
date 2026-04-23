package shell

import (
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/errfmt"
)

// repairMessageSequence scans the conversation for orphaned tool calls —
// ToolCall entries in an AssistantMessage that have no corresponding
// ToolResultMessage — and inserts synthetic placeholder results immediately
// after the originating assistant message. This handles mid-history orphans
// (crash-resume) correctly, not just end-of-run interrupts.
//
// Pure: returns the input slice unchanged when no repair is needed.
// Idempotent: synthetic results carry [error:TOOL_INTERRUPTED]; a second pass
// will find them as real results and produce no further repairs.
func repairMessageSequence(msgs []core.Message) []core.Message {
	// First pass: collect all tool call IDs that already have results.
	hasResult := make(map[string]struct{}, len(msgs))
	for _, m := range msgs {
		if tr, ok := m.(*core.ToolResultMessage); ok {
			hasResult[tr.ToolCallID] = struct{}{}
		}
	}

	// Second pass: walk in order. For each assistant message, collect its
	// orphaned tool call IDs; insert synthetics immediately after the message.
	out := make([]core.Message, 0, len(msgs))
	repaired := false

	for _, m := range msgs {
		out = append(out, m)

		am, ok := m.(*core.AssistantMessage)
		if !ok {
			continue
		}

		for _, c := range am.Content {
			tc, ok := c.(core.ToolCall)
			if !ok {
				continue
			}
			if _, found := hasResult[tc.ID]; found {
				continue
			}
			// Synthesize a result inline, right after this assistant message.
			out = append(out, &core.ToolResultMessage{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Content: []core.ContentBlock{
					core.TextContent{
						Text: "[error:" + string(errfmt.ToolErrInterrupted) + "] tool execution did not complete before session was interrupted; result unavailable",
					},
				},
				IsError:   true,
				Timestamp: time.Now(),
			})
			repaired = true
		}
	}

	if !repaired {
		return msgs
	}
	return out
}
