package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

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
	Model               string            `json:"model"`
	Messages            []oaiMessage      `json:"messages"`
	MaxTokens           *int              `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	Temperature         *float64          `json:"temperature,omitempty"`
	Stream              bool              `json:"stream"`
	Tools               []oaiTool         `json:"tools,omitempty"`
	ToolChoice          any               `json:"tool_choice,omitempty"`
	StreamOptions       *oaiStreamOptions `json:"stream_options,omitempty"`
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
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
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
	maxTokens := p.resolveMaxTokens(req)

	msgs, err := p.convertMessages(req)
	if err != nil {
		return nil, err
	}

	oaiReq := oaiRequest{
		Model:         p.model.ID,
		Messages:      msgs,
		Stream:        true,
		StreamOptions: &oaiStreamOptions{IncludeUsage: true},
	}

	// Newer OpenAI models use max_completion_tokens instead of max_tokens
	if useMaxCompletionTokens(p.model.ID) {
		oaiReq.MaxCompletionTokens = &maxTokens
	} else {
		oaiReq.MaxTokens = &maxTokens
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

func (p *OpenAI) convertMessages(req core.StreamRequest) ([]oaiMessage, error) {
	var msgs []oaiMessage
	var convErr error
	if req.System != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: req.System})
	}
	msgs = append(msgs, convertMessageList(req.Messages, messageConverters[oaiMessage]{
		User: p.convertUserMessage,
		Assistant: func(msg *core.AssistantMessage) oaiMessage {
			m, err := p.convertAssistantMessage(msg)
			if err != nil && convErr == nil {
				convErr = err
			}
			return m
		},
		ToolResult: func(msg *core.ToolResultMessage) oaiMessage {
			return oaiMessage{
				Role:       "tool",
				Content:    toolResultText(msg),
				ToolCallID: msg.ToolCallID,
			}
		},
	})...)
	return msgs, convErr
}

func (p *OpenAI) convertUserMessage(msg *core.UserMessage) oaiMessage {
	if msg.Content != "" && len(msg.Blocks) == 0 {
		return oaiMessage{Role: "user", Content: msg.Content}
	}

	blocks := decodeUserBlocks(msg,
		func(text string) map[string]any {
			return map[string]any{"type": "text", "text": text}
		},
		func(img core.ImageContent) map[string]any {
			return map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": fmt.Sprintf("data:%s;base64,%s", img.MimeType, img.Data)},
			}
		},
	)

	if len(blocks) == 0 {
		return oaiMessage{Role: "user", Content: ""}
	}
	return oaiMessage{Role: "user", Content: blocks}
}

func (p *OpenAI) convertAssistantMessage(msg *core.AssistantMessage) (oaiMessage, error) {
	var text string
	var toolCalls []oaiToolCall

	for _, c := range msg.Content {
		switch block := c.(type) {
		case core.TextContent:
			text += block.Text
		case core.ToolCall:
			argsJSON, err := json.Marshal(block.Arguments)
			if err != nil {
				return oaiMessage{}, fmt.Errorf("marshal tool arguments for %q: %w", block.Name, err)
			}
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
	return m, nil
}

func (p *OpenAI) convertTools(tools []core.ToolSchema) []oaiTool {
	return convertToolSchemas(tools, func(name, desc string, params any) oaiTool {
		return oaiTool{
			Type: "function",
			Function: oaiFunction{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		}
	})
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (p *OpenAI) endpoint() string {
	base := strings.TrimSuffix(p.model.BaseURL, "/")
	// If base URL already ends with a version path (e.g. /v4), skip /v1 prefix
	if hasVersionSuffix(base) {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

// hasVersionSuffix checks if the URL ends with /v<N> (e.g. /v1, /v4).
func hasVersionSuffix(url string) bool {
	// Find last path segment
	i := strings.LastIndex(url, "/")
	if i < 0 || i >= len(url)-1 {
		return false
	}
	seg := url[i+1:]
	if len(seg) < 2 || seg[0] != 'v' {
		return false
	}
	for _, c := range seg[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (p *OpenAI) sendRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return p.doHTTPRequest(ctx, p.endpoint(), body, func(req *http.Request) {
		req.Header.Set("Accept", "text/event-stream")
		if apiKey := p.apiKeyFn(); apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else if isLoopbackURL(p.model.BaseURL) {
			req.Header.Set("Authorization", "Bearer local")
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
	Index        int             `json:"index"`
	Delta        *oaiChoiceDelta `json:"delta"`
	FinishReason string          `json:"finish_reason"`
}

type oaiChoiceDelta struct {
	Content   string        `json:"content,omitempty"`
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
}

type oaiUsage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *oaiPromptTokensInfo `json:"prompt_tokens_details,omitempty"`
}

type oaiPromptTokensInfo struct {
	CachedTokens int `json:"cached_tokens"`
}

func (p *OpenAI) parseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage {
	var msg core.AssistantMessage
	toolArgs := make(map[int]*strings.Builder)
	textBuilders := make(map[int]*strings.Builder)

	scanSSE(ctx, reader, ch, func(data []byte) {
		var evt oaiStreamEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return
		}

		// Usage
		if evt.Usage != nil {
			msg.Usage = core.Usage{
				InputTokens:  evt.Usage.PromptTokens,
				OutputTokens: evt.Usage.CompletionTokens,
			}
			if evt.Usage.PromptTokensDetails != nil {
				msg.Usage.CacheReadTokens = evt.Usage.PromptTokensDetails.CachedTokens
			}
		}

		if len(evt.Choices) == 0 {
			return
		}

		choice := evt.Choices[0]

		// Text delta
		if choice.Delta != nil && choice.Delta.Content != "" {
			ch <- core.StreamEvent{Type: core.StreamTextDelta, Delta: choice.Delta.Content}
			appendTextBuilder(&msg, choice.Delta.Content, textBuilders)
		}

		// Tool call deltas
		if choice.Delta != nil {
			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}

				ensureToolCall(&msg, idx, tc)

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
	})

	finalizeTextBuilders(&msg, textBuilders)

	// Finalize tool call arguments
	for idx, builder := range toolArgs {
		finalizeToolArgs(&msg, idx, builder.String())
	}

	return msg
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

var oaiStopReasons = map[string]core.StopReason{
	"stop":       core.StopReasonStop,
	"length":     core.StopReasonLength,
	"tool_calls": core.StopReasonTool,
}

func mapStopReason(reason string) core.StopReason {
	return mapStopReasonFromTable(reason, oaiStopReasons)
}

var maxCompletionTokensSet = sync.OnceValue(func() map[string]bool {
	set := make(map[string]bool)
	for _, m := range CuratedModels() {
		if m.MaxCompletionTokens {
			set[m.ID] = true
		}
	}
	return set
})

// useMaxCompletionTokens returns true for models that require
// max_completion_tokens instead of max_tokens.
func useMaxCompletionTokens(modelID string) bool {
	if maxCompletionTokensSet()[modelID] {
		return true
	}
	// Fallback heuristic for non-curated models (custom endpoints, OpenRouter)
	for _, p := range []string{"o1", "o3", "o4"} {
		if strings.HasPrefix(modelID, p) {
			return true
		}
	}
	const pfx = "gpt-"
	if strings.HasPrefix(modelID, pfx) {
		rest := modelID[len(pfx):]
		if len(rest) > 0 && rest[0] >= '5' {
			return true
		}
		if strings.HasPrefix(rest, "4.1") || strings.HasPrefix(rest, "4.5") {
			return true
		}
	}
	return false
}
