package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/dotcommander/piglet/core"
)

const anthropicAPIVersion = "2023-06-01"

// Anthropic implements core.StreamProvider for the Anthropic Messages API.
type Anthropic struct {
	baseProvider
}

// NewAnthropic creates an Anthropic provider.
func NewAnthropic(model core.Model, apiKeyFn func() string) *Anthropic {
	return &Anthropic{baseProvider: newBaseProvider(model, apiKeyFn)}
}

// Stream implements core.StreamProvider.
func (p *Anthropic) Stream(ctx context.Context, req core.StreamRequest) <-chan core.StreamEvent {
	return runStream(ctx, req, p)
}

func (p *Anthropic) streamModel() core.Model { return p.model }

// ---------------------------------------------------------------------------
// Request types
// ---------------------------------------------------------------------------

type antRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    any          `json:"system,omitempty"` // string or []antSystemBlock
	Messages  []antMessage `json:"messages"`
	Stream    bool         `json:"stream"`
	Tools     []antTool    `json:"tools,omitempty"`
}

type antCacheControl struct {
	Type string `json:"type"`
}

type antSystemBlock struct {
	Type         string           `json:"type"`
	Text         string           `json:"text"`
	CacheControl *antCacheControl `json:"cache_control,omitempty"`
}

type antMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []antBlock
}

type antBlock struct {
	Type         string           `json:"type"`
	CacheControl *antCacheControl `json:"cache_control,omitempty"`
	// Text block
	Text string `json:"text,omitempty"`
	// Image block
	Source *antImageSource `json:"source,omitempty"`
	// Tool use block
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
	// Tool result block
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"` // string or []antBlock
	IsError   bool   `json:"is_error,omitempty"`
}

type antImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type antTool struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	InputSchema  any              `json:"input_schema"`
	CacheControl *antCacheControl `json:"cache_control,omitempty"`
}

func (p *Anthropic) buildRequest(req core.StreamRequest) ([]byte, error) {
	maxTokens := p.model.MaxTokens
	if req.Options.MaxTokens != nil {
		maxTokens = *req.Options.MaxTokens
	}

	antReq := antRequest{
		Model:     p.model.ID,
		MaxTokens: maxTokens,
		Messages:  p.convertMessages(req),
		Stream:    true,
	}

	// System prompt as cacheable block
	if req.System != "" {
		antReq.System = []antSystemBlock{{
			Type:         "text",
			Text:         req.System,
			CacheControl: &antCacheControl{Type: "ephemeral"},
		}}
	}

	if len(req.Tools) > 0 {
		antReq.Tools = p.convertTools(req.Tools)
		// Cache breakpoint on last tool — tools are stable within a session
		antReq.Tools[len(antReq.Tools)-1].CacheControl = &antCacheControl{Type: "ephemeral"}
	}

	return json.Marshal(antReq)
}

func (p *Anthropic) convertMessages(req core.StreamRequest) []antMessage {
	var msgs []antMessage

	for _, m := range req.Messages {
		switch msg := m.(type) {
		case *core.UserMessage:
			msgs = append(msgs, p.convertUserMessage(msg))
		case *core.AssistantMessage:
			msgs = append(msgs, p.convertAssistantMessage(msg))
		case *core.ToolResultMessage:
			msgs = append(msgs, p.convertToolResult(msg))
		}
	}

	// Add cache breakpoint on the second-to-last user-role message.
	// This enables incremental caching: on the next turn, the entire
	// conversation prefix up to this point is a cache hit.
	addConversationCacheBreakpoint(msgs)

	return msgs
}

// addConversationCacheBreakpoint marks the second-to-last user-role message
// with cache_control so the prefix is cacheable for the next turn.
func addConversationCacheBreakpoint(msgs []antMessage) {
	// Find the last two user-role messages
	var lastTwo [2]int
	count := 0
	for i := len(msgs) - 1; i >= 0 && count < 2; i-- {
		if msgs[i].Role == "user" {
			lastTwo[count] = i
			count++
		}
	}
	if count < 2 {
		return
	}

	// Mark the second-to-last user message
	idx := lastTwo[1]
	content := msgs[idx].Content
	switch c := content.(type) {
	case []antBlock:
		if len(c) > 0 {
			c[len(c)-1].CacheControl = &antCacheControl{Type: "ephemeral"}
		}
	case string:
		// Convert string to block array so we can attach cache_control
		msgs[idx].Content = []antBlock{{
			Type:         "text",
			Text:         c,
			CacheControl: &antCacheControl{Type: "ephemeral"},
		}}
	}
}

func (p *Anthropic) convertUserMessage(msg *core.UserMessage) antMessage {
	if msg.Content != "" && len(msg.Blocks) == 0 {
		return antMessage{Role: "user", Content: msg.Content}
	}

	var blocks []antBlock
	if msg.Content != "" {
		blocks = append(blocks, antBlock{Type: "text", Text: msg.Content})
	}
	for _, b := range msg.Blocks {
		switch c := b.(type) {
		case core.TextContent:
			blocks = append(blocks, antBlock{Type: "text", Text: c.Text})
		case core.ImageContent:
			blocks = append(blocks, antBlock{
				Type: "image",
				Source: &antImageSource{
					Type:      "base64",
					MediaType: c.MimeType,
					Data:      c.Data,
				},
			})
		}
	}

	return antMessage{Role: "user", Content: blocks}
}

func (p *Anthropic) convertAssistantMessage(msg *core.AssistantMessage) antMessage {
	var blocks []antBlock
	for _, c := range msg.Content {
		switch block := c.(type) {
		case core.TextContent:
			blocks = append(blocks, antBlock{Type: "text", Text: block.Text})
		case core.ToolCall:
			blocks = append(blocks, antBlock{
				Type:  "tool_use",
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Arguments,
			})
		}
	}
	return antMessage{Role: "assistant", Content: blocks}
}

func (p *Anthropic) convertToolResult(msg *core.ToolResultMessage) antMessage {
	text := toolResultText(msg)
	return antMessage{
		Role: "user",
		Content: []antBlock{{
			Type:      "tool_result",
			ToolUseID: msg.ToolCallID,
			Content:   text,
			IsError:   msg.IsError,
		}},
	}
}

func (p *Anthropic) convertTools(tools []core.ToolSchema) []antTool {
	out := make([]antTool, 0, len(tools))
	for _, t := range tools {
		schema := t.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		out = append(out, antTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (p *Anthropic) endpoint() string {
	base := strings.TrimSuffix(p.model.BaseURL, "/")
	return base + "/v1/messages"
}

func (p *Anthropic) sendRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return p.doHTTPRequest(ctx, p.endpoint(), body, func(req *http.Request) {
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("anthropic-version", anthropicAPIVersion)
		if apiKey := p.apiKeyFn(); apiKey != "" {
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

func (p *Anthropic) parseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage {
	var msg core.AssistantMessage
	toolArgs := make(map[int]*strings.Builder)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- core.StreamEvent{Type: core.StreamError, Error: ctx.Err()}
			return msg
		default:
		}

		line := scanner.Text()
		data := extractSSEData(line)
		if data == "" {
			continue
		}

		var evt antStreamEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
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
					continue
				}

				switch delta.Type {
				case "text_delta":
					ch <- core.StreamEvent{Type: core.StreamTextDelta, Index: evt.Index, Delta: delta.Text}
					p.appendTextAtIndex(&msg, evt.Index, delta.Text)
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
				if evt.Usage.CacheCreationInputTokens > 0 {
					msg.Usage.CacheWriteTokens = evt.Usage.CacheCreationInputTokens
				}
				if evt.Usage.CacheReadInputTokens > 0 {
					msg.Usage.CacheReadTokens = evt.Usage.CacheReadInputTokens
				}
			}
		}
	}

	// Finalize tool arguments
	for idx, builder := range toolArgs {
		p.finalizeToolArgs(&msg, idx, builder.String())
	}

	return msg
}

func (p *Anthropic) appendTextAtIndex(msg *core.AssistantMessage, idx int, delta string) {
	if idx < len(msg.Content) {
		if tc, ok := msg.Content[idx].(core.TextContent); ok {
			msg.Content[idx] = core.TextContent{Text: tc.Text + delta}
		}
	}
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

func (p *Anthropic) mapStopReason(reason string) core.StopReason {
	switch reason {
	case "end_turn":
		return core.StopReasonStop
	case "max_tokens":
		return core.StopReasonLength
	case "tool_use":
		return core.StopReasonTool
	default:
		return core.StopReasonStop
	}
}
