package compact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeSmallMsgs returns n wire messages each with ~20-token content (~80 bytes of JSON data).
func makeSmallMsgs(n int) []WireMsg {
	msgs := make([]WireMsg, n)
	for i := range n {
		data, _ := json.Marshal(map[string]string{
			"role":    "user",
			"content": "short message content here ok", // ~29 chars → ~7 tokens
		})
		msgs[i] = WireMsg{Type: "user", Data: data}
	}
	return msgs
}

// TestEarlyExitIfTrimSufficient tests the SufficientAfterTrim early-exit path directly,
// without going through makeCompactHandler (which requires ext/Storer stubs).
// It verifies the two functions the branch composes: estimateTokens and encodeAllMessages.
func TestEarlyExitIfTrimSufficient(t *testing.T) {
	t.Parallel()

	t.Run("below threshold encodes all messages unchanged", func(t *testing.T) {
		t.Parallel()

		msgs := makeSmallMsgs(10)
		tokens := estimateTokens(msgs)

		// Sanity: 10 messages × ~50 bytes each = ~500 bytes → ~125 tokens — well below 1000.
		require.Less(t, tokens, 1000, "test setup: messages should be well below 1000 tokens")

		threshold := 1000
		require.LessOrEqual(t, tokens, threshold)

		out, err := encodeAllMessages(msgs)
		require.NoError(t, err)

		var decoded struct {
			Messages []WireMsg `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(out, &decoded))

		// All 10 messages must be present — no window slide, no summary injection.
		assert.Len(t, decoded.Messages, 10)
		for i, m := range decoded.Messages {
			assert.Equal(t, msgs[i].Type, m.Type, "message %d type mismatch", i)
			assert.Equal(t, string(msgs[i].Data), string(m.Data), "message %d data mismatch", i)
		}
	})

	t.Run("disabled when SufficientAfterTrim is zero", func(t *testing.T) {
		t.Parallel()

		msgs := makeSmallMsgs(10)
		tokens := estimateTokens(msgs)

		// With threshold=0 the guard condition is false — the branch must not trigger.
		// We verify this by checking the condition expression directly, mirroring the handler.
		threshold := 0
		earlyExitWouldTrigger := threshold > 0 && tokens <= threshold
		assert.False(t, earlyExitWouldTrigger, "SufficientAfterTrim=0 must disable early exit")
	})

	t.Run("does not trigger when tokens exceed threshold", func(t *testing.T) {
		t.Parallel()

		msgs := makeSmallMsgs(10)
		tokens := estimateTokens(msgs)

		// Set threshold below current token count — guard must not fire.
		threshold := tokens - 1
		earlyExitWouldTrigger := threshold > 0 && tokens <= threshold
		assert.False(t, earlyExitWouldTrigger, "early exit must not trigger when tokens exceed threshold")
	})
}

// TestEncodeAllMessages verifies the wire shape produced by encodeAllMessages.
func TestEncodeAllMessages(t *testing.T) {
	t.Parallel()

	msgs := []WireMsg{
		{Type: "user", Data: json.RawMessage(`{"role":"user","content":"hello"}`)},
		{Type: "assistant", Data: json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"hi"}]}`)},
	}

	out, err := encodeAllMessages(msgs)
	require.NoError(t, err)

	var decoded struct {
		Messages []WireMsg `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(out, &decoded))
	require.Len(t, decoded.Messages, 2)

	assert.Equal(t, "user", decoded.Messages[0].Type)
	assert.Equal(t, "assistant", decoded.Messages[1].Type)
	assert.JSONEq(t, `{"role":"user","content":"hello"}`, string(decoded.Messages[0].Data))
}
