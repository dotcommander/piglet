package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/dotcommander/piglet/core"
	pigletprovider "github.com/dotcommander/piglet/provider"
)

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (p *Anthropic) endpoint() string {
	base := strings.TrimSuffix(p.Model.BaseURL, "/")
	return base + "/v1/messages"
}

func (p *Anthropic) SendRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return p.DoHTTPRequest(ctx, p.endpoint(), body, func(req *http.Request) {
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("anthropic-version", anthropicAPIVersion)
		if apiKey := p.APIKeyFn(); apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
		}
	})
}

// ---------------------------------------------------------------------------
// SSE parsing
// ---------------------------------------------------------------------------

type antStreamEvent struct {
	Type  string          `json:"type"`
	Index int             `json:"index"`
	Delta json.RawMessage `json:"delta,omitempty"`

	// content_block_start
	ContentBlock *antContentBlock `json:"content_block,omitempty"`

	// message_start
	Message *antStreamMessage `json:"message,omitempty"`

	// message_delta
	Usage *antStreamUsage `json:"usage,omitempty"`
}

type antContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

type antStreamMessage struct {
	Usage *antStreamUsage `json:"usage,omitempty"`
}

type antStreamUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type antDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

func (p *Anthropic) ParseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage {
	var msg core.AssistantMessage
	toolArgs := make(map[int]*strings.Builder)
	textBuilders := make(map[int]*strings.Builder)

	pigletprovider.ScanSSE(ctx, reader, ch, func(data []byte) {
		var evt antStreamEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return
		}

		switch evt.Type {
		case "message_start":
			if evt.Message != nil && evt.Message.Usage != nil {
				msg.Usage.InputTokens = evt.Message.Usage.InputTokens
				msg.Usage.CacheWriteTokens = evt.Message.Usage.CacheCreationInputTokens
				msg.Usage.CacheReadTokens = evt.Message.Usage.CacheReadInputTokens
			}

		case "content_block_start":
			if evt.ContentBlock != nil {
				switch evt.ContentBlock.Type {
				case "text":
					msg.Content = append(msg.Content, core.TextContent{})
					textBuilders[evt.Index] = &strings.Builder{}
				case "tool_use":
					msg.Content = append(msg.Content, core.ToolCall{
						ID:        evt.ContentBlock.ID,
						Name:      evt.ContentBlock.Name,
						Arguments: map[string]any{},
					})
					toolArgs[evt.Index] = &strings.Builder{}
				}
			}

		case "content_block_delta":
			if evt.Delta != nil {
				var delta antDelta
				if err := json.Unmarshal(evt.Delta, &delta); err != nil {
					return
				}

				switch delta.Type {
				case "text_delta":
					ch <- core.StreamEvent{Type: core.StreamTextDelta, Index: evt.Index, Delta: delta.Text}
					if b, ok := textBuilders[evt.Index]; ok {
						b.WriteString(delta.Text)
					}
				case "input_json_delta":
					ch <- core.StreamEvent{Type: core.StreamToolCallDelta, Index: evt.Index, Delta: delta.PartialJSON}
					if builder, ok := toolArgs[evt.Index]; ok {
						builder.WriteString(delta.PartialJSON)
					}
				}
			}

		case "message_delta":
			if evt.Delta != nil {
				var delta antDelta
				if err := json.Unmarshal(evt.Delta, &delta); err == nil && delta.StopReason != "" {
					msg.StopReason = p.mapStopReason(delta.StopReason)
				}
			}
			if evt.Usage != nil {
				msg.Usage.OutputTokens = evt.Usage.OutputTokens
				msg.Usage.CacheWriteTokens = evt.Usage.CacheCreationInputTokens
				msg.Usage.CacheReadTokens = evt.Usage.CacheReadInputTokens
			}
		}
	})

	// Finalize accumulated text
	pigletprovider.FinalizeTextBuilders(&msg, textBuilders)

	// Finalize tool arguments
	for idx, builder := range toolArgs {
		p.finalizeToolArgs(&msg, idx, builder.String())
	}

	return msg
}

func (p *Anthropic) finalizeToolArgs(msg *core.AssistantMessage, idx int, argsJSON string) {
	if idx < len(msg.Content) {
		if tc, ok := msg.Content[idx].(core.ToolCall); ok {
			var args map[string]any
			if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
				tc.Arguments = args
			}
			msg.Content[idx] = tc
		}
	}
}

var antStopReasons = map[string]core.StopReason{
	"end_turn":   core.StopReasonStop,
	"max_tokens": core.StopReasonLength,
	"tool_use":   core.StopReasonTool,
}

func (p *Anthropic) mapStopReason(reason string) core.StopReason {
	return pigletprovider.MapStopReasonFromTable(reason, antStopReasons)
}
