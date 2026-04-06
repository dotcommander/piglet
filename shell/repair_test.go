package shell

import (
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	tr, ok := result[3].(*core.ToolResultMessage)
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
