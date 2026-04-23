package sdk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildJSON marshals v to json.RawMessage, failing the test on error.
func buildJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestDecodeEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		decode func(t *testing.T, raw json.RawMessage)
	}{
		{
			name: "EventAgentStart",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventAgentStartPayload](raw)
				require.NoError(t, err)
				_ = p // no fields to assert
			},
		},
		{
			name: "EventAgentEnd",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventAgentEndPayload](raw)
				require.NoError(t, err)
				assert.NotNil(t, p.Messages)
			},
		},
		{
			name: "EventTurnStart",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventTurnStartPayload](raw)
				require.NoError(t, err)
				_ = p
			},
		},
		{
			name: "EventTurnEnd",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventTurnEndPayload](raw)
				require.NoError(t, err)
				assert.NotNil(t, p.Assistant)
			},
		},
		{
			name: "EventStreamDelta",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventStreamDeltaPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, "text", p.Kind)
				assert.Equal(t, 2, p.Index)
				assert.Equal(t, "hello", p.Delta)
			},
		},
		{
			name: "EventStreamDone",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventStreamDonePayload](raw)
				require.NoError(t, err)
				assert.NotNil(t, p.Message)
			},
		},
		{
			name: "EventToolStart",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventToolStartPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, "call-1", p.ToolCallID)
				assert.Equal(t, "bash", p.ToolName)
				assert.Equal(t, "ls -la", p.Args["command"])
			},
		},
		{
			name: "EventToolUpdate",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventToolUpdatePayload](raw)
				require.NoError(t, err)
				assert.Equal(t, "call-2", p.ToolCallID)
				assert.Equal(t, "read", p.ToolName)
				assert.NotNil(t, p.Partial)
			},
		},
		{
			name: "EventToolEnd",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventToolEndPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, "call-3", p.ToolCallID)
				assert.Equal(t, "write", p.ToolName)
				assert.True(t, p.IsError)
			},
		},
		{
			name: "EventRetry",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventRetryPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, 2, p.Attempt)
				assert.Equal(t, 3, p.Max)
				assert.Equal(t, 500, p.DelayMs)
				assert.Equal(t, "rate limited", p.Error)
			},
		},
		{
			name: "EventMaxTurns",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventMaxTurnsPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, 10, p.Count)
				assert.Equal(t, 10, p.Max)
			},
		},
		{
			name: "EventStepWait",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventStepWaitPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, "call-4", p.ToolCallID)
				assert.Equal(t, "bash", p.ToolName)
				assert.Equal(t, "rm -rf /tmp/x", p.Args["command"])
			},
		},
		{
			name: "EventCompact",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventCompactPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, 120, p.Before)
				assert.Equal(t, 40, p.After)
				assert.Equal(t, 95000, p.TokensAtCompact)
			},
		},
		{
			name: "EventSessionLoad",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventSessionLoadPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, 7, p.MessageCount)
			},
		},
		{
			name: "EventAgentInit",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventAgentInitPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, 12, p.ToolCount)
			},
		},
		{
			name: "EventPromptBuild",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventPromptBuildPayload](raw)
				require.NoError(t, err)
				assert.Equal(t, "You are a helpful assistant.", p.System)
			},
		},
		{
			name: "EventMessagePre",
			decode: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				p, err := DecodeEvent[EventMessagePrePayload](raw)
				require.NoError(t, err)
				assert.Equal(t, "hello piglet", p.Content)
			},
		},
	}

	// Per-event sample payloads built programmatically via map → json.Marshal.
	payloads := map[string]json.RawMessage{
		"EventAgentStart": buildJSON(t, map[string]any{}),
		"EventAgentEnd": buildJSON(t, map[string]any{
			"Messages": []any{
				map[string]any{"content": "hi", "timestamp": "2024-01-01T00:00:00Z"},
			},
		}),
		"EventTurnStart": buildJSON(t, map[string]any{}),
		"EventTurnEnd": buildJSON(t, map[string]any{
			"Assistant": map[string]any{
				"content":    []any{},
				"model":      "gpt-4o",
				"provider":   "openai",
				"usage":      map[string]any{},
				"stopReason": "stop",
				"timestamp":  "2024-01-01T00:00:00Z",
			},
			"ToolResults": nil,
		}),
		"EventStreamDelta": buildJSON(t, map[string]any{
			"Kind":  "text",
			"Index": 2,
			"Delta": "hello",
		}),
		"EventStreamDone": buildJSON(t, map[string]any{
			"Message": map[string]any{
				"content":    []any{},
				"model":      "gpt-4o",
				"provider":   "openai",
				"usage":      map[string]any{},
				"stopReason": "stop",
				"timestamp":  "2024-01-01T00:00:00Z",
			},
		}),
		"EventToolStart": buildJSON(t, map[string]any{
			"ToolCallID": "call-1",
			"ToolName":   "bash",
			"Args":       map[string]any{"command": "ls -la"},
		}),
		"EventToolUpdate": buildJSON(t, map[string]any{
			"ToolCallID": "call-2",
			"ToolName":   "read",
			"Partial":    map[string]any{"lines": 10},
		}),
		"EventToolEnd": buildJSON(t, map[string]any{
			"ToolCallID": "call-3",
			"ToolName":   "write",
			"Result":     nil,
			"IsError":    true,
		}),
		"EventRetry": buildJSON(t, map[string]any{
			"Attempt": 2,
			"Max":     3,
			"DelayMs": 500,
			"Error":   "rate limited",
		}),
		"EventMaxTurns": buildJSON(t, map[string]any{
			"Count": 10,
			"Max":   10,
		}),
		"EventStepWait": buildJSON(t, map[string]any{
			"ToolCallID": "call-4",
			"ToolName":   "bash",
			"Args":       map[string]any{"command": "rm -rf /tmp/x"},
		}),
		"EventCompact": buildJSON(t, map[string]any{
			"Before":          120,
			"After":           40,
			"TokensAtCompact": 95000,
		}),
		"EventSessionLoad": buildJSON(t, map[string]any{
			"MessageCount": 7,
		}),
		"EventAgentInit": buildJSON(t, map[string]any{
			"ToolCount": 12,
		}),
		"EventPromptBuild": buildJSON(t, map[string]any{
			"System": "You are a helpful assistant.",
		}),
		"EventMessagePre": buildJSON(t, map[string]any{
			"Content": "hello piglet",
		}),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, ok := payloads[tc.name]
			require.Truef(t, ok, "missing payload for %s", tc.name)
			tc.decode(t, raw)
		})
	}
}

// TestDecodeEventError verifies DecodeEvent returns an error on invalid JSON.
func TestDecodeEventError(t *testing.T) {
	t.Parallel()
	_, err := DecodeEvent[EventRetryPayload](json.RawMessage(`not-json`))
	assert.Error(t, err)
}
