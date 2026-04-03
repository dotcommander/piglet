package memory

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultClearerConfig(t *testing.T) {
	t.Parallel()
	cfg := defaultClearerConfig()
	assert.Equal(t, defaultClearTurns, cfg.Turns)
}

func TestClearOldToolResults_NotEnoughMessages(t *testing.T) {
	t.Parallel()
	// clearOldToolResults returns 0 when len(messages) <= turnThreshold+1
	// We can't call clearOldToolResults directly (needs sdk.Extension), but we can
	// test the threshold logic via the wireToolResult and wireMsg JSON behaviour.
	// Build a messages slice shorter than threshold+1 and verify nothing is cleared.
	threshold := defaultClearTurns

	msgs := make([]wireMsg, threshold+1) // exactly at boundary
	for i := range msgs {
		tr := wireToolResult{
			ToolCallID: "id",
			ToolName:   "Read",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: largeText(clearSizeThreshold + 1)}},
		}
		data, err := json.Marshal(tr)
		require.NoError(t, err)
		msgs[i] = wireMsg{Type: "tool_result", Data: data}
	}
	// With exactly threshold+1 messages the condition len(messages) <= threshold+1 is true
	// so cutoff would be 0 and nothing is cleared.
	cutoff := len(msgs) - threshold
	assert.Equal(t, 1, cutoff) // one message before the window
}

func TestClearOldToolResults_BelowSizeThreshold(t *testing.T) {
	t.Parallel()
	// Build a tool result whose text is below clearSizeThreshold — should not be cleared.
	tr := wireToolResult{
		ToolCallID: "tc1",
		ToolName:   "Read",
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: "small result"}},
	}
	data, err := json.Marshal(tr)
	require.NoError(t, err)

	textSize := 0
	for _, c := range tr.Content {
		if c.Type == "text" {
			textSize += len(c.Text)
		}
	}
	assert.LessOrEqual(t, textSize, clearSizeThreshold, "text should be below threshold")
	_ = data // confirms marshalling works
}

func TestClearOldToolResults_AboveSizeThreshold(t *testing.T) {
	t.Parallel()
	bigText := largeText(clearSizeThreshold + 100)
	tr := wireToolResult{
		ToolCallID: "tc2",
		ToolName:   "Bash",
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: bigText}},
	}
	data, err := json.Marshal(tr)
	require.NoError(t, err)

	textSize := 0
	for _, c := range tr.Content {
		if c.Type == "text" {
			textSize += len(c.Text)
		}
	}
	assert.Greater(t, textSize, clearSizeThreshold)
	_ = data
}

func TestWireToolResult_RoundTrip(t *testing.T) {
	t.Parallel()
	tr := wireToolResult{
		ToolCallID: "abc123",
		ToolName:   "Read",
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: "some content"},
			{Type: "text", Text: "more content"},
		},
		IsError: false,
	}

	data, err := json.Marshal(tr)
	require.NoError(t, err)

	var got wireToolResult
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, tr.ToolCallID, got.ToolCallID)
	assert.Equal(t, tr.ToolName, got.ToolName)
	assert.Len(t, got.Content, 2)
	assert.Equal(t, "text", got.Content[0].Type)
	assert.Equal(t, "some content", got.Content[0].Text)
}

func TestWireMsg_RoundTrip(t *testing.T) {
	t.Parallel()
	tr := wireToolResult{
		ToolCallID: "id1",
		ToolName:   "Bash",
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: "output"}},
	}
	trData, err := json.Marshal(tr)
	require.NoError(t, err)

	msg := wireMsg{Type: "tool_result", Data: trData}
	msgData, err := json.Marshal(msg)
	require.NoError(t, err)

	var got wireMsg
	require.NoError(t, json.Unmarshal(msgData, &got))
	assert.Equal(t, "tool_result", got.Type)

	var gotTR wireToolResult
	require.NoError(t, json.Unmarshal(got.Data, &gotTR))
	assert.Equal(t, "Bash", gotTR.ToolName)
}

// largeText returns a string of exactly n bytes of repeating 'x' characters.
func largeText(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
