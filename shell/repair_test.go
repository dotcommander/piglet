package shell

import (
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- legacy tests (preserved) -----------------------------------------------

func TestRepairMessageSequence_NoOrphans(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.UserMessage{Content: "hello", Timestamp: time.Now()},
		&core.AssistantMessage{
			Content: []core.AssistantContent{
				core.ToolCall{ID: "tc1", Name: "bash", Arguments: map[string]any{"cmd": "ls"}},
			},
		},
		&core.ToolResultMessage{
			ToolCallID: "tc1",
			ToolName:   "bash",
			Content:    []core.ContentBlock{core.TextContent{Text: "file.go"}},
		},
	}

	result := repairMessageSequence(msgs)
	assert.Len(t, result, 3)
	// Same slice — no copy needed
	assert.Equal(t, msgs, result)
}

func TestRepairMessageSequence_OrphanedToolCall(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.UserMessage{Content: "hello", Timestamp: time.Now()},
		&core.AssistantMessage{
			Content: []core.AssistantContent{
				core.ToolCall{ID: "tc1", Name: "bash", Arguments: map[string]any{"cmd": "ls"}},
				core.ToolCall{ID: "tc2", Name: "read", Arguments: map[string]any{"path": "/tmp/x"}},
			},
		},
		&core.ToolResultMessage{
			ToolCallID: "tc1",
			ToolName:   "bash",
			Content:    []core.ContentBlock{core.TextContent{Text: "file.go"}},
		},
		// tc2 is orphaned — no ToolResultMessage
	}

	result := repairMessageSequence(msgs)
	require.Len(t, result, 4) // original 3 + 1 synthesized

	// New algorithm inserts the synthetic immediately after the assistant
	// message (index 1), so the real tc1 result shifts to index 3.
	// Layout: [0] user, [1] asst, [2] synthetic(tc2), [3] real(tc1)
	tr, ok := result[2].(*core.ToolResultMessage)
	require.True(t, ok)
	assert.Equal(t, "tc2", tr.ToolCallID)
	assert.Equal(t, "read", tr.ToolName)
	assert.True(t, tr.IsError)
}

func TestRepairMessageSequence_MultipleOrphans(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.AssistantMessage{
			Content: []core.AssistantContent{
				core.ToolCall{ID: "a", Name: "tool1"},
				core.ToolCall{ID: "b", Name: "tool2"},
				core.ToolCall{ID: "c", Name: "tool3"},
			},
		},
	}

	result := repairMessageSequence(msgs)
	require.Len(t, result, 4) // 1 original + 3 repairs
	for _, m := range result[1:] {
		tr, ok := m.(*core.ToolResultMessage)
		require.True(t, ok)
		assert.True(t, tr.IsError)
	}
}

func TestRepairMessageSequence_Empty(t *testing.T) {
	t.Parallel()
	result := repairMessageSequence(nil)
	assert.Nil(t, result)
}

func TestRepairMessageSequence_NoToolCalls(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.UserMessage{Content: "hi"},
		&core.AssistantMessage{Content: []core.AssistantContent{core.TextContent{Text: "hello"}}},
	}

	result := repairMessageSequence(msgs)
	assert.Len(t, result, 2)
}

// --- new spec tests ----------------------------------------------------------

// clean_no_dangling: assistant with matched tool_use → return unchanged.
func TestRepairMessageSequence_CleanNoDangling(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.AssistantMessage{
			Content: []core.AssistantContent{
				core.ToolCall{ID: "x1", Name: "bash"},
			},
		},
		&core.ToolResultMessage{
			ToolCallID: "x1",
			Content:    []core.ContentBlock{core.TextContent{Text: "ok"}},
		},
	}

	result := repairMessageSequence(msgs)
	assert.Len(t, result, 2)
	// Identical slice pointer proves no copy was made.
	assert.Equal(t, msgs, result)
}

// dangling_at_tail: assistant with tool_use, no following tool_result →
// one synthetic ToolResultMessage appended.
func TestRepairMessageSequence_DanglingAtTail(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.AssistantMessage{
			Content: []core.AssistantContent{
				core.ToolCall{ID: "t1", Name: "bash"},
			},
		},
	}

	result := repairMessageSequence(msgs)
	require.Len(t, result, 2)
	tr, ok := result[1].(*core.ToolResultMessage)
	require.True(t, ok, "index 1 must be a ToolResultMessage")
	assert.Equal(t, "t1", tr.ToolCallID)
	assert.True(t, tr.IsError)
	require.Len(t, tr.Content, 1)
	text := tr.Content[0].(core.TextContent).Text
	assert.True(t, strings.HasPrefix(text, "[error:TOOL_INTERRUPTED]"), "content must start with [error:TOOL_INTERRUPTED], got: %q", text)
}

// dangling_mid_conversation: assistant A (tool_use_1, matched), assistant B
// (tool_use_2, dangling), then follow-up user message → synthetic result
// inserted between B and the user message.
func TestRepairMessageSequence_DanglingMidConversation(t *testing.T) {
	t.Parallel()

	user1 := &core.UserMessage{Content: "do something"}
	assistantA := &core.AssistantMessage{
		Content: []core.AssistantContent{
			core.ToolCall{ID: "a1", Name: "bash"},
		},
	}
	resultA := &core.ToolResultMessage{
		ToolCallID: "a1",
		Content:    []core.ContentBlock{core.TextContent{Text: "done"}},
	}
	assistantB := &core.AssistantMessage{
		Content: []core.AssistantContent{
			core.ToolCall{ID: "b1", Name: "read"},
		},
	}
	user2 := &core.UserMessage{Content: "follow-up"}

	msgs := []core.Message{user1, assistantA, resultA, assistantB, user2}

	result := repairMessageSequence(msgs)
	// Expected: user1, assistantA, resultA, assistantB, synthetic-for-b1, user2
	require.Len(t, result, 6)
	assert.Equal(t, assistantB, result[3])

	synth, ok := result[4].(*core.ToolResultMessage)
	require.True(t, ok, "index 4 must be synthetic ToolResultMessage")
	assert.Equal(t, "b1", synth.ToolCallID)
	assert.True(t, synth.IsError)
	text := synth.Content[0].(core.TextContent).Text
	assert.True(t, strings.HasPrefix(text, "[error:TOOL_INTERRUPTED]"))

	assert.Equal(t, user2, result[5], "user2 must follow the synthetic result")
}

// partial_match: assistant with 3 tool_use (a, b, c), only a and c have
// results → synthetic for b inserted after the assistant, before a's result.
func TestRepairMessageSequence_PartialMatch(t *testing.T) {
	t.Parallel()

	asst := &core.AssistantMessage{
		Content: []core.AssistantContent{
			core.ToolCall{ID: "a", Name: "tool_a"},
			core.ToolCall{ID: "b", Name: "tool_b"},
			core.ToolCall{ID: "c", Name: "tool_c"},
		},
	}
	resultA := &core.ToolResultMessage{ToolCallID: "a", Content: []core.ContentBlock{core.TextContent{Text: "ra"}}}
	resultC := &core.ToolResultMessage{ToolCallID: "c", Content: []core.ContentBlock{core.TextContent{Text: "rc"}}}

	msgs := []core.Message{asst, resultA, resultC}

	result := repairMessageSequence(msgs)
	// Expected: asst, synthetic-b, resultA, resultC
	require.Len(t, result, 4)
	assert.Equal(t, asst, result[0])

	synth, ok := result[1].(*core.ToolResultMessage)
	require.True(t, ok, "index 1 must be synthetic ToolResultMessage for b")
	assert.Equal(t, "b", synth.ToolCallID)
	assert.True(t, synth.IsError)
	text := synth.Content[0].(core.TextContent).Text
	assert.True(t, strings.HasPrefix(text, "[error:TOOL_INTERRUPTED]"))

	assert.Equal(t, resultA, result[2])
	assert.Equal(t, resultC, result[3])
}

// idempotent: run repair twice → second run returns same slice, no extra synthetics.
func TestRepairMessageSequence_Idempotent(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.AssistantMessage{
			Content: []core.AssistantContent{
				core.ToolCall{ID: "z1", Name: "bash"},
			},
		},
	}

	first := repairMessageSequence(msgs)
	require.Len(t, first, 2, "first repair must add one synthetic")

	second := repairMessageSequence(first)
	assert.Len(t, second, 2, "second repair must not add more synthetics")
	// Returned unchanged — same slice identity.
	assert.Equal(t, first, second)
}
