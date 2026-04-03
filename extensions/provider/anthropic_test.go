package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseLines builds a mock SSE stream from data payloads.
func sseLines(lines ...string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	return b.String()
}

func TestAnthropic_StreamText(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "sk-ant-test", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_start","message":{"usage":{"input_tokens":42,"output_tokens":0,"cache_creation_input_tokens":5,"cache_read_input_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7,"cache_creation_input_tokens":5,"cache_read_input_tokens":0}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID:        "claude-haiku-test",
		Provider:  "anthropic",
		API:       core.APIAnthropic,
		BaseURL:   server.URL,
		MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "sk-ant-test" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		System:   "Be helpful.",
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
	assert.Equal(t, 42, finalMsg.Usage.InputTokens)
	assert.Equal(t, 7, finalMsg.Usage.OutputTokens)
	assert.Equal(t, 5, finalMsg.Usage.CacheWriteTokens)
}

func TestAnthropic_StreamToolCall(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_start","message":{"usage":{"input_tokens":20,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"calculator"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"x\":"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"42}"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":10}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID:        "claude-haiku-test",
		Provider:  "anthropic",
		API:       core.APIAnthropic,
		BaseURL:   server.URL,
		MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "Calculate", Timestamp: time.Now()}},
		Tools:    []core.ToolSchema{{Name: "calculator", Description: "Does math"}},
	})

	var toolDeltas []string
	var finalMsg *core.AssistantMessage
	for evt := range ch {
		switch evt.Type {
		case core.StreamToolCallDelta:
			toolDeltas = append(toolDeltas, evt.Delta)
		case core.StreamDone:
			finalMsg = evt.Message
		case core.StreamError:
			t.Fatalf("unexpected error: %v", evt.Error)
		}
	}

	require.NotNil(t, finalMsg)
	assert.Equal(t, core.StopReasonTool, finalMsg.StopReason)
	require.Len(t, finalMsg.Content, 1)

	tc, ok := finalMsg.Content[0].(core.ToolCall)
	require.True(t, ok)
	assert.Equal(t, "toolu_01", tc.ID)
	assert.Equal(t, "calculator", tc.Name)
	assert.Equal(t, float64(42), tc.Arguments["x"])
	assert.Equal(t, []string{"{\"x\":", "42}"}, toolDeltas)
}

func TestAnthropic_StreamHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"type":"authentication_error","message":"Invalid API Key"}}`)
	}))
	defer server.Close()

	model := core.Model{
		ID:        "claude-haiku-test",
		Provider:  "anthropic",
		API:       core.APIAnthropic,
		BaseURL:   server.URL,
		MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "bad" })

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

func TestAnthropic_StreamRateLimitError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`)
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "test", Timestamp: time.Now()}},
	})

	var gotError bool
	for evt := range ch {
		if evt.Type == core.StreamError {
			gotError = true
			assert.Contains(t, evt.Error.Error(), "429")
		}
	}
	assert.True(t, gotError)
}

func TestAnthropic_StreamMaxTokensStopReason(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_start","message":{"usage":{"input_tokens":10}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Truncated"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":3}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 5,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "tell me a long story", Timestamp: time.Now()}},
	})

	var finalMsg *core.AssistantMessage
	for evt := range ch {
		if evt.Type == core.StreamDone {
			finalMsg = evt.Message
		}
	}

	require.NotNil(t, finalMsg)
	assert.Equal(t, core.StopReasonLength, finalMsg.StopReason)
}

func TestAnthropic_StreamCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.CloseClientConnections()
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch := prov.Stream(ctx, core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "test", Timestamp: time.Now()}},
	})

	for range ch {
	}
}

func TestAnthropic_StreamWithCustomMaxTokens(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		))
	}))
	defer server.Close()

	maxTok := 512
	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 2048,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
		Options:  core.StreamOptions{MaxTokens: &maxTok},
	})
	for range ch {
	}

	assert.Contains(t, string(capturedBody), `"max_tokens":512`)
}

func TestAnthropic_StreamMultipleContentBlocks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_start","message":{"usage":{"input_tokens":10}}}`,
			// First text block
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"First"}}`,
			// Tool use block
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_02","name":"search"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"hello\"}"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":15}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "search", Timestamp: time.Now()}},
		Tools:    []core.ToolSchema{{Name: "search", Description: "web search"}},
	})

	var finalMsg *core.AssistantMessage
	for evt := range ch {
		if evt.Type == core.StreamDone {
			finalMsg = evt.Message
		}
	}

	require.NotNil(t, finalMsg)
	require.Len(t, finalMsg.Content, 2)

	_, isText := finalMsg.Content[0].(core.TextContent)
	assert.True(t, isText, "first block should be TextContent")

	tc, ok := finalMsg.Content[1].(core.ToolCall)
	require.True(t, ok, "second block should be ToolCall")
	assert.Equal(t, "toolu_02", tc.ID)
	assert.Equal(t, "search", tc.Name)
}

func TestAnthropic_StreamEndpointURL(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})
	for range ch {
	}

	assert.Equal(t, "/v1/messages", gotPath)
}

func TestAnthropic_StreamSystemPromptCached(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		System:   "You are an expert assistant.",
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})
	for range ch {
	}

	// System prompt should be sent as an array with cache_control
	body := string(capturedBody)
	assert.Contains(t, body, `"system"`)
	assert.Contains(t, body, `"cache_control"`)
	assert.Contains(t, body, `"ephemeral"`)
}

func TestAnthropic_BuildRequest_ToolsCached(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
		Tools: []core.ToolSchema{
			{Name: "tool_a", Description: "first tool"},
			{Name: "tool_b", Description: "last tool"},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	// Last tool should have cache_control
	assert.Contains(t, body, `"cache_control"`)
	// Both tools should be present
	assert.Contains(t, body, `"tool_a"`)
	assert.Contains(t, body, `"tool_b"`)
}

func TestAnthropic_StreamCacheReadTokens(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_start","message":{"usage":{"input_tokens":5,"cache_read_input_tokens":100}}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3,"cache_read_input_tokens":100}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})

	var finalMsg *core.AssistantMessage
	for evt := range ch {
		if evt.Type == core.StreamDone {
			finalMsg = evt.Message
		}
	}

	require.NotNil(t, finalMsg)
	assert.Equal(t, 100, finalMsg.Usage.CacheReadTokens)
}

func TestAnthropic_StreamMessageMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_start","message":{"usage":{"input_tokens":10}}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})

	var finalMsg *core.AssistantMessage
	for evt := range ch {
		if evt.Type == core.StreamDone {
			finalMsg = evt.Message
		}
	}

	require.NotNil(t, finalMsg)
	// runStream sets Model, Provider, and Timestamp
	assert.Equal(t, "claude-test", finalMsg.Model)
	assert.Equal(t, "anthropic", finalMsg.Provider)
	assert.False(t, finalMsg.Timestamp.IsZero())
}

func TestAnthropic_ConversationHistory(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	now := time.Now()
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{Content: "first", Timestamp: now},
			&core.AssistantMessage{
				Content:    []core.AssistantContent{core.TextContent{Text: "response"}},
				StopReason: core.StopReasonStop,
				Timestamp:  now,
			},
			&core.UserMessage{Content: "second", Timestamp: now},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"first"`)
	assert.Contains(t, body, `"response"`)
	assert.Contains(t, body, `"second"`)
	// With 2+ user messages, cache breakpoint should be added on second-to-last user msg
	assert.Contains(t, body, `"cache_control"`)
}

func TestAnthropic_ToolResultMessage(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "claude-test", Provider: "anthropic", API: core.APIAnthropic,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewAnthropic(model, func() string { return "key" })

	now := time.Now()
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{Content: "run the tool", Timestamp: now},
			&core.AssistantMessage{
				Content: []core.AssistantContent{core.ToolCall{
					ID: "toolu_01", Name: "calc", Arguments: map[string]any{"x": 1},
				}},
				StopReason: core.StopReasonTool,
				Timestamp:  now,
			},
			&core.ToolResultMessage{
				ToolCallID: "toolu_01",
				ToolName:   "calc",
				Content:    []core.ContentBlock{core.TextContent{Text: "42"}},
				Timestamp:  now,
			},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"tool_result"`)
	assert.Contains(t, body, `"toolu_01"`)
	assert.Contains(t, body, `"42"`)
}
