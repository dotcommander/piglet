package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dotcommander/piglet/core"
)

// OpenAI implements core.StreamProvider for OpenAI-compatible APIs.
type OpenAI struct {
	baseProvider
}

// NewOpenAI creates a provider for OpenAI-compatible APIs.
func NewOpenAI(model core.Model, apiKeyFn func() string) *OpenAI {
	return &OpenAI{baseProvider: newBaseProvider(model, apiKeyFn)}
}

// Stream implements core.StreamProvider.
func (p *OpenAI) Stream(ctx context.Context, req core.StreamRequest) <-chan core.StreamEvent {
	return runStream(ctx, req, p)
}

func (p *OpenAI) streamModel() core.Model { return p.model }

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

type oaiRequest struct {
	Model         string            `json:"model"`
	Messages      []oaiMessage      `json:"messages"`
	MaxTokens     *int              `json:"max_tokens,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	Stream        bool              `json:"stream"`
	Tools         []oaiTool         `json:"tools,omitempty"`
	ToolChoice    any               `json:"tool_choice,omitempty"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type oaiToolCall struct {
	Index    *int            `json:"index,omitempty"`
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiFunctionCall `json:"function"`
}

type oaiFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

func (p *OpenAI) buildRequest(req core.StreamRequest) ([]byte, error) {
	maxTokens := p.model.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	oaiReq := oaiRequest{
		Model:         p.model.ID,
		Messages:      p.convertMessages(req),
		MaxTokens:     &maxTokens,
		Stream:        true,
		StreamOptions: &oaiStreamOptions{IncludeUsage: true},
	}

	if req.Options.Temperature != nil {
		oaiReq.Temperature = req.Options.Temperature
	}

	if len(req.Tools) > 0 {
		oaiReq.Tools = p.convertTools(req.Tools)
		oaiReq.ToolChoice = "auto"
	}

	return json.Marshal(oaiReq)
}

func (p *OpenAI) convertMessages(req core.StreamRequest) []oaiMessage {
	var msgs []oaiMessage

	// System message
	if req.System != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: req.System})
	}

	for _, m := range req.Messages {
		switch msg := m.(type) {
		case *core.UserMessage:
			msgs = append(msgs, p.convertUserMessage(msg))
		case *core.AssistantMessage:
			msgs = append(msgs, p.convertAssistantMessage(msg))
		case *core.ToolResultMessage:
			msgs = append(msgs, oaiMessage{
				Role:       "tool",
				Content:    toolResultText(msg),
				ToolCallID: msg.ToolCallID,
			})
		}
	}

	return msgs
}

func (p *OpenAI) convertUserMessage(msg *core.UserMessage) oaiMessage {
	if msg.Content != "" && len(msg.Blocks) == 0 {
		return oaiMessage{Role: "user", Content: msg.Content}
	}

	var blocks []map[string]any
	if msg.Content != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
	}
	for _, b := range msg.Blocks {
		switch c := b.(type) {
		case core.TextContent:
			blocks = append(blocks, map[string]any{"type": "text", "text": c.Text})
		case core.ImageContent:
			blocks = append(blocks, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": fmt.Sprintf("data:%s;base64,%s", c.MimeType, c.Data)},
			})
		}
	}

	if len(blocks) == 0 {
		return oaiMessage{Role: "user", Content: ""}
	}
	return oaiMessage{Role: "user", Content: blocks}
}

func (p *OpenAI) convertAssistantMessage(msg *core.AssistantMessage) oaiMessage {
	var text string
	var toolCalls []oaiToolCall

	for _, c := range msg.Content {
		switch block := c.(type) {
		case core.TextContent:
			text += block.Text
		case core.ToolCall:
			argsJSON, _ := json.Marshal(block.Arguments)
			toolCalls = append(toolCalls, oaiToolCall{
				ID:   block.ID,
				Type: "function",
				Function: oaiFunctionCall{
					Name:      block.Name,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	m := oaiMessage{Role: "assistant"}
	if text != "" {
		m.Content = text
	}
	if len(toolCalls) > 0 {
		m.ToolCalls = toolCalls
	}
	return m
}

func (p *OpenAI) convertTools(tools []core.ToolSchema) []oaiTool {
	oaiTools := make([]oaiTool, 0, len(tools))
	for _, t := range tools {
		params, _ := t.Parameters.(map[string]any)
		if params == nil {
			params = map[string]any{"type": "object"}
		}
		oaiTools = append(oaiTools, oaiTool{
			Type: "function",
			Function: oaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return oaiTools
}

func toolResultText(msg *core.ToolResultMessage) string {
	var parts []string
	for _, b := range msg.Content {
		if tc, ok := b.(core.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (p *OpenAI) endpoint() string {
	base := strings.TrimSuffix(p.model.BaseURL, "/")
	return base + "/v1/chat/completions"
}

func (p *OpenAI) sendRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return p.doHTTPRequest(ctx, p.endpoint(), body, func(req *http.Request) {
		req.Header.Set("Accept", "text/event-stream")
		if apiKey := p.apiKeyFn(); apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	})
}

// ---------------------------------------------------------------------------
// SSE Stream parsing
// ---------------------------------------------------------------------------

type oaiStreamEvent struct {
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage,omitempty"`
}

type oaiChoice struct {
	Index        int              `json:"index"`
	Delta        *oaiChoiceDelta  `json:"delta"`
	FinishReason string           `json:"finish_reason"`
}

type oaiChoiceDelta struct {
	Content   string        `json:"content,omitempty"`
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (p *OpenAI) parseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage {
	var msg core.AssistantMessage
	toolArgs := make(map[int]*strings.Builder) // index → accumulated args JSON

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
		if data == "" || data == "[DONE]" {
			continue
		}

		var evt oaiStreamEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}

		// Usage
		if evt.Usage != nil {
			msg.Usage = core.Usage{
				InputTokens:  evt.Usage.PromptTokens,
				OutputTokens: evt.Usage.CompletionTokens,
			}
		}

		if len(evt.Choices) == 0 {
			continue
		}

		choice := evt.Choices[0]

		// Text delta
		if choice.Delta != nil && choice.Delta.Content != "" {
			ch <- core.StreamEvent{Type: core.StreamTextDelta, Delta: choice.Delta.Content}
			appendText(&msg, choice.Delta.Content)
		}

		// Tool call deltas
		if choice.Delta != nil {
			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}

				// Ensure tool call exists in message content
				ensureToolCall(&msg, idx, tc)

				// Accumulate arguments
				if tc.Function.Arguments != "" {
					if _, ok := toolArgs[idx]; !ok {
						toolArgs[idx] = &strings.Builder{}
					}
					toolArgs[idx].WriteString(tc.Function.Arguments)

					ch <- core.StreamEvent{
						Type:  core.StreamToolCallDelta,
						Index: idx,
						Delta: tc.Function.Arguments,
					}
				}
			}
		}

		// Finish reason
		if choice.FinishReason != "" {
			msg.StopReason = mapStopReason(choice.FinishReason)
		}
	}

	// Finalize tool call arguments
	for idx, builder := range toolArgs {
		finalizeToolArgs(&msg, idx, builder.String())
	}

	return msg
}

func extractSSEData(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "data: ") {
		return strings.TrimPrefix(trimmed, "data: ")
	}
	if strings.HasPrefix(trimmed, "data:") {
		return strings.TrimPrefix(trimmed, "data:")
	}
	// Some providers send raw JSON
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return trimmed
	}
	return ""
}

func appendText(msg *core.AssistantMessage, delta string) {
	for i := range msg.Content {
		if tc, ok := msg.Content[i].(core.TextContent); ok {
			msg.Content[i] = core.TextContent{Text: tc.Text + delta}
			return
		}
	}
	msg.Content = append(msg.Content, core.TextContent{Text: delta})
}

func ensureToolCall(msg *core.AssistantMessage, idx int, tc oaiToolCall) {
	// Find existing tool call at this index
	toolIdx := 0
	for i, c := range msg.Content {
		if _, ok := c.(core.ToolCall); ok {
			if toolIdx == idx {
				// Update name/ID if provided
				existing := msg.Content[i].(core.ToolCall)
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Name = tc.Function.Name
				}
				msg.Content[i] = existing
				return
			}
			toolIdx++
		}
	}

	// Create new tool call
	msg.Content = append(msg.Content, core.ToolCall{
		ID:        tc.ID,
		Name:      tc.Function.Name,
		Arguments: map[string]any{},
	})
}

func finalizeToolArgs(msg *core.AssistantMessage, idx int, argsJSON string) {
	toolIdx := 0
	for i, c := range msg.Content {
		if tc, ok := c.(core.ToolCall); ok {
			if toolIdx == idx {
				var args map[string]any
				if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
					tc.Arguments = args
				}
				msg.Content[i] = tc
				return
			}
			toolIdx++
		}
	}
}

func mapStopReason(reason string) core.StopReason {
	switch reason {
	case "stop":
		return core.StopReasonStop
	case "length":
		return core.StopReasonLength
	case "tool_calls":
		return core.StopReasonTool
	default:
		return core.StopReasonStop
	}
}
