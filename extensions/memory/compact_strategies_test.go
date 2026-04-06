package memory

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMicrocompactToolResults(t *testing.T) {
	t.Parallel()

	t.Run("replaces tool results outside keepRecent", func(t *testing.T) {
		t.Parallel()

		tr := wireToolResult{
			ToolName: "bash",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "lots of output here that is very long"}},
		}
		trData, err := json.Marshal(tr)
		require.NoError(t, err)

		msgs := []wireMsg{
			{Type: "tool_result", Data: trData},
			{Type: "assistant", Data: json.RawMessage(`{"content":"ok"}`)},
			{Type: "user", Data: json.RawMessage(`{"content":"next"}`)},
		}

		microcompactToolResults(msgs, 2)

		var got wireToolResult
		require.NoError(t, json.Unmarshal(msgs[0].Data, &got))
		assert.Equal(t, "[bash: 37 chars]", got.Content[0].Text)
		assert.Len(t, got.Content, 1)
	})

	t.Run("preserves tool results within keepRecent", func(t *testing.T) {
		t.Parallel()

		tr := wireToolResult{
			ToolName: "read",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "file contents"}},
		}
		trData, err := json.Marshal(tr)
		require.NoError(t, err)

		msgs := []wireMsg{
			{Type: "user", Data: json.RawMessage(`{"content":"hi"}`)},
			{Type: "tool_result", Data: trData},
		}

		microcompactToolResults(msgs, 2)

		var got wireToolResult
		require.NoError(t, json.Unmarshal(msgs[1].Data, &got))
		assert.Equal(t, "file contents", got.Content[0].Text)
	})

	t.Run("marks error tool results", func(t *testing.T) {
		t.Parallel()

		tr := wireToolResult{
			ToolName: "bash",
			IsError:  true,
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "command failed"}},
		}
		trData, err := json.Marshal(tr)
		require.NoError(t, err)

		msgs := []wireMsg{
			{Type: "tool_result", Data: trData},
			{Type: "user", Data: json.RawMessage(`{"content":"next"}`)},
			{Type: "assistant", Data: json.RawMessage(`{"content":"ok"}`)},
		}

		microcompactToolResults(msgs, 2)

		var got wireToolResult
		require.NoError(t, json.Unmarshal(msgs[0].Data, &got))
		assert.Equal(t, "[bash: error, 14 chars]", got.Content[0].Text)
	})

	t.Run("noop when all within keepRecent", func(t *testing.T) {
		t.Parallel()

		msgs := []wireMsg{
			{Type: "user", Data: json.RawMessage(`{"content":"hi"}`)},
		}
		orig := string(msgs[0].Data)
		microcompactToolResults(msgs, 5)
		assert.Equal(t, orig, string(msgs[0].Data))
	})
}

func TestLightTrimMessages(t *testing.T) {
	t.Parallel()

	t.Run("trims long user message outside keepRecent", func(t *testing.T) {
		t.Parallel()

		longContent := make([]rune, 200)
		for i := range longContent {
			longContent[i] = 'a'
		}
		data, err := json.Marshal(map[string]string{
			"role":    "user",
			"content": string(longContent),
		})
		require.NoError(t, err)

		msgs := []wireMsg{
			{Type: "user", Data: data},
			{Type: "assistant", Data: json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`)},
			{Type: "user", Data: json.RawMessage(`{"content":"recent"}`)},
		}

		lightTrimMessages(msgs, 2, 100) // maxLen=100, half=50

		var m struct {
			Content string `json:"content"`
		}
		require.NoError(t, json.Unmarshal(msgs[0].Data, &m))
		assert.Contains(t, m.Content, "[...trimmed for compaction...]")
		assert.Less(t, len([]rune(m.Content)), 200)
	})

	t.Run("trims long assistant text block", func(t *testing.T) {
		t.Parallel()

		longText := make([]rune, 300)
		for i := range longText {
			longText[i] = 'b'
		}

		content := []map[string]string{
			{"type": "text", "text": string(longText)},
		}
		data, err := json.Marshal(map[string]any{
			"role":    "assistant",
			"content": content,
		})
		require.NoError(t, err)

		msgs := []wireMsg{
			{Type: "assistant", Data: data},
			{Type: "user", Data: json.RawMessage(`{"content":"recent1"}`)},
			{Type: "assistant", Data: json.RawMessage(`{"content":[{"type":"text","text":"recent2"}]}`)},
		}

		lightTrimMessages(msgs, 2, 100)

		var m struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		require.NoError(t, json.Unmarshal(msgs[0].Data, &m))
		require.Len(t, m.Content, 1)
		assert.Contains(t, m.Content[0].Text, "[...trimmed for compaction...]")
		assert.Less(t, len([]rune(m.Content[0].Text)), 300)
	})

	t.Run("preserves short messages", func(t *testing.T) {
		t.Parallel()

		data, err := json.Marshal(map[string]string{
			"role":    "user",
			"content": "short",
		})
		require.NoError(t, err)

		msgs := []wireMsg{
			{Type: "user", Data: data},
			{Type: "assistant", Data: json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`)},
			{Type: "user", Data: json.RawMessage(`{"content":"recent"}`)},
		}

		orig := string(msgs[0].Data)
		lightTrimMessages(msgs, 2, 100)
		assert.Equal(t, orig, string(msgs[0].Data))
	})

	t.Run("noop when all within keepRecent", func(t *testing.T) {
		t.Parallel()

		msgs := []wireMsg{
			{Type: "user", Data: json.RawMessage(`{"content":"hi"}`)},
		}
		orig := string(msgs[0].Data)
		lightTrimMessages(msgs, 5, 100)
		assert.Equal(t, orig, string(msgs[0].Data))
	})

	t.Run("noop with zero maxLen", func(t *testing.T) {
		t.Parallel()

		msgs := []wireMsg{
			{Type: "user", Data: json.RawMessage(`{"content":"hi"}`)},
			{Type: "user", Data: json.RawMessage(`{"content":"there"}`)},
		}
		orig := string(msgs[0].Data)
		lightTrimMessages(msgs, 1, 0)
		assert.Equal(t, orig, string(msgs[0].Data))
	})
}

func TestEstimateTokens(t *testing.T) {
	t.Parallel()

	msgs := []wireMsg{
		{Type: "user", Data: json.RawMessage(`{"content":"hello world"}`)},                    // 26 bytes
		{Type: "assistant", Data: json.RawMessage(`{"content":[{"type":"text","text":""}]}`)}, // 39 bytes
	}

	tokens := estimateTokens(msgs)
	// (26 + 39) / 4 = 16
	assert.Equal(t, 16, tokens)
}

func TestEstimateTokens_Empty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, estimateTokens(nil))
}
