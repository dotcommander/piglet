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
	BaseProvider
}

// NewOpenAI creates a provider for OpenAI-compatible APIs.
func NewOpenAI(model core.Model, apiKeyFn func() string) *OpenAI {
	return &OpenAI{BaseProvider: NewBaseProvider(model, apiKeyFn)}
}

// Stream implements core.StreamProvider.
func (p *OpenAI) Stream(ctx context.Context, req core.StreamRequest) <-chan core.StreamEvent {
	return RunStream(ctx, req, p)
}

func (p *OpenAI) StreamModel() core.Model { return p.Model }

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

func (p *OpenAI) BuildRequest(req core.StreamRequest) ([]byte, error) {
	maxTokens := p.ResolveMaxTokens(req)

	msgs, err := p.convertMessages(req)
	if err != nil {
		return nil, err
	}

	oaiReq := oaiRequest{
		Model:         p.Model.ID,
		Messages:      msgs,
		Stream:        true,
		StreamOptions: &oaiStreamOptions{IncludeUsage: true},
	}

	// Newer OpenAI models use max_completion_tokens instead of max_tokens
	if useMaxCompletionTokens(p.Model.ID) {
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
	msgs = append(msgs, ConvertMessageList(req.Messages, MessageConverters[oaiMessage]{
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
				Content:    ToolResultText(msg),
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

	blocks := DecodeUserBlocks(msg,
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
	return ConvertToolSchemas(tools, func(name, desc string, params any) oaiTool {
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
	base := strings.TrimSuffix(p.Model.BaseURL, "/")
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

func (p *OpenAI) SendRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return p.DoHTTPRequest(ctx, p.endpoint(), body, func(req *http.Request) {
		req.Header.Set("Accept", "text/event-stream")
		if apiKey := p.APIKeyFn(); apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else if IsLoopbackURL(p.Model.BaseURL) {
			req.Header.Set("Authorization", "Bearer local")
		}
	})
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
	// Fallback heuristic for non-curated models (custom endpoints, OpenRouter):
	// check against the prefix list loaded from models.yaml.
	for _, pfx := range MaxCompletionTokensPrefixes() {
		if strings.HasPrefix(modelID, pfx) {
			return true
		}
	}
	return false
}
