package autotitle

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExtractFirstExchange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		messages      []json.RawMessage
		wantUser      string
		wantAssistant string
	}{
		{
			name:          "empty",
			messages:      nil,
			wantUser:      "",
			wantAssistant: "",
		},
		{
			name: "user_only",
			messages: []json.RawMessage{
				mustJSON(t, testMsg{Role: "user", Content: "hello"}),
			},
			wantUser:      "hello",
			wantAssistant: "",
		},
		{
			name: "assistant_only",
			messages: []json.RawMessage{
				mustJSON(t, testMsg{Role: "assistant", Content: "hi there"}),
			},
			wantUser:      "",
			wantAssistant: "hi there",
		},
		{
			name: "user_then_assistant",
			messages: []json.RawMessage{
				mustJSON(t, testMsg{Role: "user", Content: "what is 2+2?"}),
				mustJSON(t, testMsg{Role: "assistant", Content: "4"}),
			},
			wantUser:      "what is 2+2?",
			wantAssistant: "4",
		},
		{
			name: "assistant_then_user",
			messages: []json.RawMessage{
				mustJSON(t, testMsg{Role: "assistant", Content: "previous response"}),
				mustJSON(t, testMsg{Role: "user", Content: "new question"}),
			},
			wantUser:      "new question",
			wantAssistant: "previous response",
		},
		{
			name: "multiple_exchanges_takes_first",
			messages: []json.RawMessage{
				mustJSON(t, testMsg{Role: "user", Content: "first user"}),
				mustJSON(t, testMsg{Role: "assistant", Content: "first assistant"}),
				mustJSON(t, testMsg{Role: "user", Content: "second user"}),
				mustJSON(t, testMsg{Role: "assistant", Content: "second assistant"}),
			},
			wantUser:      "first user",
			wantAssistant: "first assistant",
		},
		{
			name: "invalid_json_skipped",
			messages: []json.RawMessage{
				json.RawMessage(`{invalid}`),
				mustJSON(t, testMsg{Role: "user", Content: "valid user"}),
				mustJSON(t, testMsg{Role: "assistant", Content: "valid assistant"}),
			},
			wantUser:      "valid user",
			wantAssistant: "valid assistant",
		},
		{
			name: "system_message_ignored",
			messages: []json.RawMessage{
				mustJSON(t, testMsg{Role: "system", Content: "system prompt"}),
				mustJSON(t, testMsg{Role: "user", Content: "hello"}),
				mustJSON(t, testMsg{Role: "assistant", Content: "hi"}),
			},
			wantUser:      "hello",
			wantAssistant: "hi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotUser, gotAssistant := extractFirstExchange(tt.messages)
			assert.Equal(t, tt.wantUser, gotUser)
			assert.Equal(t, tt.wantAssistant, gotAssistant)
		})
	}
}

func TestTruncateTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{"empty", "", 10, ""},
		{"shorter_than_limit", "hello", 10, "hello"},
		{"exact_limit", "hello", 5, "hello"},
		{"longer_than_limit", "hello world", 5, "hello"},
		{"unicode_preserved", "日本語テスト", 3, "日本語"},
		{"zero_limit", "hello", 0, ""},
		{"single_rune", "a", 1, "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateTitle(tt.input, tt.limit)
			assert.Equal(t, tt.want, got)
		})
	}
}
