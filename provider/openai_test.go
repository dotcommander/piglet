package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/provider"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sseLines(lines ...string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func TestOpenAI_StreamText(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
			`{"choices":[{"index":0,"delta":{"content":" world"}}]}`,
			`{"choices":[{"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID:       "test-model",
		Provider: "test",
		API:      core.APIOpenAI,
		BaseURL:  server.URL,
	}
	prov := provider.NewOpenAI(model, func() string { return "sk-test" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		System:   "You are helpful.",
		Messages: []core.Message{&core.UserMessage{Content: "Hi", Timestamp: time.Now()}},
	})

	var deltas []string
	var finalMsg *core.AssistantMessage
	for evt := range ch {
		switch evt.Type {
		case core.StreamTextDelta:
			deltas = append(deltas, evt.Delta)
		case core.StreamDone:
			finalMsg = evt.Message
		case core.StreamError:
			t.Fatalf("unexpected error: %v", evt.Error)
		}
	}

	assert.Equal(t, []string{"Hello", " world"}, deltas)
	require.NotNil(t, finalMsg)
	require.Len(t, finalMsg.Content, 1)
	assert.Equal(t, "Hello world", finalMsg.Content[0].(core.TextContent).Text)
	assert.Equal(t, core.StopReasonStop, finalMsg.StopReason)
	assert.Equal(t, 10, finalMsg.Usage.InputTokens)
	assert.Equal(t, 5, finalMsg.Usage.OutputTokens)
}

func TestOpenAI_StreamToolCall(t *testing.T) {
	t.Parallel()

	idx0 := 0
	_ = idx0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"tc1","type":"function","function":{"name":"echo","arguments":""}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"","type":"","function":{"arguments":"{\"text\":"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"","type":"","function":{"arguments":"\"hello\"}"}}]}}]}`,
			`{"choices":[{"index":0,"finish_reason":"tool_calls"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{ID: "test", Provider: "test", API: core.APIOpenAI, BaseURL: server.URL}
	prov := provider.NewOpenAI(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "test", Timestamp: time.Now()}},
		Tools:    []core.ToolSchema{{Name: "echo", Description: "echo tool"}},
	})

	var msg *core.AssistantMessage
	for evt := range ch {
		if evt.Type == core.StreamDone {
			msg = evt.Message
		}
	}

	require.NotNil(t, msg)
	assert.Equal(t, core.StopReasonTool, msg.StopReason)
	require.Len(t, msg.Content, 1)

	tc, ok := msg.Content[0].(core.ToolCall)
	require.True(t, ok)
	assert.Equal(t, "tc1", tc.ID)
	assert.Equal(t, "echo", tc.Name)
	assert.Equal(t, "hello", tc.Arguments["text"])
}

func TestOpenAI_StreamHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer server.Close()

	model := core.Model{ID: "test", Provider: "test", API: core.APIOpenAI, BaseURL: server.URL}
	prov := provider.NewOpenAI(model, func() string { return "bad-key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "test", Timestamp: time.Now()}},
	})

	var gotError bool
	for evt := range ch {
		if evt.Type == core.StreamError {
			gotError = true
			assert.Contains(t, evt.Error.Error(), "401")
		}
	}
	assert.True(t, gotError)
}

func TestOpenAI_StreamCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Send one delta then hang
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		// Block until client disconnects
		<-r.Context().Done()
	}))
	defer server.Close()

	model := core.Model{ID: "test", Provider: "test", API: core.APIOpenAI, BaseURL: server.URL}
	prov := provider.NewOpenAI(model, func() string { return "key" })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch := prov.Stream(ctx, core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "test", Timestamp: time.Now()}},
	})

	// Drain — should complete due to context cancellation
	for range ch {
	}
}
