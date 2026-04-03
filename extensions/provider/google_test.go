package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoogle_StreamText(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "alt=sse")
		assert.Equal(t, "goog-test", r.Header.Get("x-goog-api-key"))

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1}}`,
			`{"candidates":[{"content":{"role":"model","parts":[{"text":" world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID:        "gemini-test",
		Provider:  "google",
		API:       core.APIGoogle,
		BaseURL:   server.URL,
		MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "goog-test" })

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
	assert.Equal(t, 10, finalMsg.Usage.InputTokens)
	assert.Equal(t, 2, finalMsg.Usage.OutputTokens)
}

func TestGoogle_StreamToolCall(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"search","args":{"query":"cats"}}}]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "search cats", Timestamp: time.Now()}},
		Tools:    []core.ToolSchema{{Name: "search", Description: "web search"}},
	})

	var toolCallEnd *core.ToolCall
	var finalMsg *core.AssistantMessage
	for evt := range ch {
		switch evt.Type {
		case core.StreamToolCallEnd:
			toolCallEnd = evt.Tool
		case core.StreamDone:
			finalMsg = evt.Message
		case core.StreamError:
			t.Fatalf("unexpected error: %v", evt.Error)
		}
	}

	require.NotNil(t, finalMsg)
	require.NotNil(t, toolCallEnd)
	assert.Equal(t, "search", toolCallEnd.Name)
	assert.Equal(t, "cats", toolCallEnd.Arguments["query"])
	require.Len(t, finalMsg.Content, 1)
	tc, ok := finalMsg.Content[0].(core.ToolCall)
	require.True(t, ok)
	assert.Equal(t, "search", tc.Name)
	assert.Equal(t, "cats", tc.Arguments["query"])
}

func TestGoogle_StreamHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"API key not valid"}}`)
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "bad-key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "test", Timestamp: time.Now()}},
	})

	var gotError bool
	for evt := range ch {
		if evt.Type == core.StreamError {
			gotError = true
			assert.Contains(t, evt.Error.Error(), "403")
		}
	}
	assert.True(t, gotError)
}

func TestGoogle_StreamMaxTokensStopReason(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Truncated"}]},"finishReason":"MAX_TOKENS"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 5,
	}
	prov := NewGoogle(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "long story", Timestamp: time.Now()}},
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

func TestGoogle_StreamSafetyStopReason(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"SAFETY"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "test", Timestamp: time.Now()}},
	})

	var finalMsg *core.AssistantMessage
	for evt := range ch {
		if evt.Type == core.StreamDone {
			finalMsg = evt.Message
		}
	}

	require.NotNil(t, finalMsg)
	assert.Equal(t, core.StopReasonError, finalMsg.StopReason)
}

func TestGoogle_StreamCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hi\"}]}}]}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.CloseClientConnections()
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch := prov.Stream(ctx, core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "test", Timestamp: time.Now()}},
	})

	for range ch {
	}
}

func TestGoogle_StreamEndpointURL(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotQuery string
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotHeader = r.Header.Get("x-goog-api-key")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-2.0-flash", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 100,
	}
	prov := NewGoogle(model, func() string { return "mykey" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})
	for range ch {
	}

	assert.Equal(t, "/v1beta/models/gemini-2.0-flash:streamGenerateContent", gotPath)
	assert.Contains(t, gotQuery, "alt=sse")
	assert.Equal(t, "mykey", gotHeader)
}

func TestGoogle_BuildRequest_SystemInstruction(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		System:   "You are a helpful assistant.",
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"systemInstruction"`)
	assert.Contains(t, body, "You are a helpful assistant.")
}

func TestGoogle_BuildRequest_Tools(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
		Tools: []core.ToolSchema{
			{Name: "my_tool", Description: "does something", Parameters: map[string]any{"type": "object"}},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"functionDeclarations"`)
	assert.Contains(t, body, `"my_tool"`)
}

func TestGoogle_BuildRequest_MaxTokens(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	customMax := 256
	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{&core.UserMessage{Content: "hi", Timestamp: time.Now()}},
		Options:  core.StreamOptions{MaxTokens: &customMax},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"maxOutputTokens":256`)
}

func TestGoogle_StreamImageBlock(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Nice image"}]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{
				Content: "describe this",
				Blocks: []core.ContentBlock{
					core.ImageContent{MimeType: "image/png", Data: "abc123"},
				},
				Timestamp: time.Now(),
			},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"inlineData"`)
	assert.Contains(t, body, `"image/png"`)
	assert.Contains(t, body, `"abc123"`)
}

func TestGoogle_StreamToolResult(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Done"}]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	now := time.Now()
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{Content: "use the tool", Timestamp: now},
			&core.AssistantMessage{
				Content: []core.AssistantContent{core.ToolCall{
					ID: "call_1", Name: "my_func", Arguments: map[string]any{"arg": "val"},
				}},
				StopReason: core.StopReasonTool,
				Timestamp:  now,
			},
			&core.ToolResultMessage{
				ToolCallID: "call_1",
				ToolName:   "my_func",
				Content:    []core.ContentBlock{core.TextContent{Text: "result_value"}},
				Timestamp:  now,
			},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	assert.Contains(t, body, `"functionResponse"`)
	assert.Contains(t, body, `"my_func"`)
	assert.Contains(t, body, `"result_value"`)
}

func TestGoogle_StreamToolErrorResult(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Sorry"}]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	now := time.Now()
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{Content: "use the tool", Timestamp: now},
			&core.AssistantMessage{
				Content: []core.AssistantContent{core.ToolCall{
					ID: "call_1", Name: "my_func", Arguments: map[string]any{},
				}},
				StopReason: core.StopReasonTool,
				Timestamp:  now,
			},
			&core.ToolResultMessage{
				ToolCallID: "call_1",
				ToolName:   "my_func",
				Content:    []core.ContentBlock{core.TextContent{Text: "it failed"}},
				IsError:    true,
				Timestamp:  now,
			},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	// Error results map to {"error": text}
	assert.Contains(t, body, `"error"`)
	assert.Contains(t, body, `"it failed"`)
}

func TestGoogle_StreamAssistantMessage(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Yes"}]},"finishReason":"STOP"}]}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

	now := time.Now()
	ch := prov.Stream(context.Background(), core.StreamRequest{
		Messages: []core.Message{
			&core.UserMessage{Content: "hello", Timestamp: now},
			&core.AssistantMessage{
				Content:    []core.AssistantContent{core.TextContent{Text: "hi there"}},
				StopReason: core.StopReasonStop,
				Timestamp:  now,
			},
			&core.UserMessage{Content: "how are you", Timestamp: now},
		},
	})
	for range ch {
	}

	body := string(capturedBody)
	// Google uses "model" role for assistant
	assert.Contains(t, body, `"model"`)
	assert.Contains(t, body, `"hi there"`)
}

func TestGoogle_StreamMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseLines(
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Done"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":5,"cachedContentTokenCount":10}}`,
		))
	}))
	defer server.Close()

	model := core.Model{
		ID: "gemini-test", Provider: "google", API: core.APIGoogle,
		BaseURL: server.URL, MaxTokens: 1024,
	}
	prov := NewGoogle(model, func() string { return "key" })

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
	assert.Equal(t, 20, finalMsg.Usage.InputTokens)
	assert.Equal(t, 5, finalMsg.Usage.OutputTokens)
	assert.Equal(t, 10, finalMsg.Usage.CacheReadTokens)
	// runStream sets Model, Provider, Timestamp
	assert.Equal(t, "gemini-test", finalMsg.Model)
	assert.Equal(t, "google", finalMsg.Provider)
	assert.False(t, finalMsg.Timestamp.IsZero())
}
