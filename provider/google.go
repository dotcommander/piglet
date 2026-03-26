package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dotcommander/piglet/core"
)

// Google implements core.StreamProvider for Google Generative AI.
type Google struct {
	baseProvider
}

// NewGoogle creates a Google provider.
func NewGoogle(model core.Model, apiKeyFn func() string) *Google {
	return &Google{baseProvider: newBaseProvider(model, apiKeyFn)}
}

// Stream implements core.StreamProvider.
func (p *Google) Stream(ctx context.Context, req core.StreamRequest) <-chan core.StreamEvent {
	return runStream(ctx, req, p)
}

func (p *Google) streamModel() core.Model { return p.model }

// ---------------------------------------------------------------------------
// Request types
// ---------------------------------------------------------------------------

type gemRequest struct {
	Contents         []gemContent      `json:"contents"`
	SystemInstruct   *gemContent       `json:"systemInstruction,omitempty"`
	Tools            []gemTool         `json:"tools,omitempty"`
	GenerationConfig *gemGenConfig     `json:"generationConfig,omitempty"`
}

type gemContent struct {
	Role  string    `json:"role"`
	Parts []gemPart `json:"parts"`
}

type gemPart struct {
	Text         string          `json:"text,omitempty"`
	InlineData   *gemInlineData  `json:"inlineData,omitempty"`
	FunctionCall *gemFuncCall    `json:"functionCall,omitempty"`
	FunctionResp *gemFuncResp    `json:"functionResponse,omitempty"`
}

type gemInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type gemFuncCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type gemFuncResp struct {
	Name     string `json:"name"`
	Response any    `json:"response"`
}

type gemTool struct {
	FunctionDeclarations []gemFuncDecl `json:"functionDeclarations"`
}

type gemFuncDecl struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type gemGenConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

func (p *Google) buildRequest(req core.StreamRequest) ([]byte, error) {
	gemReq := gemRequest{
		Contents: p.convertMessages(req),
		GenerationConfig: &gemGenConfig{
			MaxOutputTokens: p.resolveMaxTokens(req),
		},
	}

	if req.System != "" {
		gemReq.SystemInstruct = &gemContent{
			Parts: []gemPart{{Text: req.System}},
		}
	}

	if req.Options.Temperature != nil {
		gemReq.GenerationConfig.Temperature = req.Options.Temperature
	}

	if len(req.Tools) > 0 {
		gemReq.Tools = p.convertTools(req.Tools)
	}

	return json.Marshal(gemReq)
}

func (p *Google) convertMessages(req core.StreamRequest) []gemContent {
	return convertMessageList(req.Messages, messageConverters[gemContent]{
		User:       p.convertUserMessage,
		Assistant:  p.convertAssistantMessage,
		ToolResult: p.convertToolResult,
	})
}

func (p *Google) convertUserMessage(msg *core.UserMessage) gemContent {
	parts := decodeUserBlocks(msg,
		func(text string) gemPart { return gemPart{Text: text} },
		func(img core.ImageContent) gemPart {
			return gemPart{InlineData: &gemInlineData{MimeType: img.MimeType, Data: img.Data}}
		},
	)

	if len(parts) == 0 {
		parts = append(parts, gemPart{Text: ""})
	}
	return gemContent{Role: "user", Parts: parts}
}

func (p *Google) convertAssistantMessage(msg *core.AssistantMessage) gemContent {
	var parts []gemPart
	for _, c := range msg.Content {
		switch block := c.(type) {
		case core.TextContent:
			parts = append(parts, gemPart{Text: block.Text})
		case core.ToolCall:
			parts = append(parts, gemPart{
				FunctionCall: &gemFuncCall{Name: block.Name, Args: block.Arguments},
			})
		}
	}
	return gemContent{Role: "model", Parts: parts}
}

func (p *Google) convertToolResult(msg *core.ToolResultMessage) gemContent {
	text := toolResultText(msg)
	resp := map[string]any{"result": text}
	if msg.IsError {
		resp = map[string]any{"error": text}
	}
	return gemContent{
		Role: "user",
		Parts: []gemPart{{
			FunctionResp: &gemFuncResp{Name: msg.ToolName, Response: resp},
		}},
	}
}

func (p *Google) convertTools(tools []core.ToolSchema) []gemTool {
	decls := convertToolSchemas(tools, func(name, desc string, params any) gemFuncDecl {
		return gemFuncDecl{
			Name:        name,
			Description: desc,
			Parameters:  params,
		}
	})
	return []gemTool{{FunctionDeclarations: decls}}
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (p *Google) endpoint() string {
	base := strings.TrimSuffix(p.model.BaseURL, "/")
	return fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", base, p.model.ID)
}

func (p *Google) sendRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	url := p.endpoint()
	if apiKey := p.apiKeyFn(); apiKey != "" {
		url += "&key=" + apiKey
	}
	return p.doHTTPRequest(ctx, url, body, nil)
}

// ---------------------------------------------------------------------------
// Stream parsing
// ---------------------------------------------------------------------------

type gemResponse struct {
	Candidates    []gemCandidate `json:"candidates"`
	UsageMetadata *gemUsage      `json:"usageMetadata,omitempty"`
}

type gemCandidate struct {
	Content       gemContent `json:"content"`
	FinishReason  string     `json:"finishReason"`
}

type gemUsage struct {
	PromptTokenCount          int `json:"promptTokenCount"`
	CandidatesTokenCount      int `json:"candidatesTokenCount"`
	CachedContentTokenCount   int `json:"cachedContentTokenCount"`
}

func (p *Google) parseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage {
	var msg core.AssistantMessage

	scanSSE(ctx, reader, ch, func(data string) {
		var resp gemResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			return
		}

		// Usage
		if resp.UsageMetadata != nil {
			msg.Usage = core.Usage{
				InputTokens:     resp.UsageMetadata.PromptTokenCount,
				OutputTokens:    resp.UsageMetadata.CandidatesTokenCount,
				CacheReadTokens: resp.UsageMetadata.CachedContentTokenCount,
			}
		}

		if len(resp.Candidates) == 0 {
			return
		}

		candidate := resp.Candidates[0]

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				ch <- core.StreamEvent{Type: core.StreamTextDelta, Delta: part.Text}
				appendText(&msg, part.Text)
			}

			if part.FunctionCall != nil {
				tc := core.ToolCall{
					ID:        fmt.Sprintf("call_%d", len(msg.Content)),
					Name:      part.FunctionCall.Name,
					Arguments: part.FunctionCall.Args,
				}
				msg.Content = append(msg.Content, tc)
				ch <- core.StreamEvent{
					Type: core.StreamToolCallEnd,
					Tool: &tc,
				}
			}
		}

		if candidate.FinishReason != "" {
			msg.StopReason = p.mapFinishReason(candidate.FinishReason)
		}
	}, withLargeBuffer(256*1024, 10*1024*1024))

	return msg
}

var gemStopReasons = map[string]core.StopReason{
	"STOP":       core.StopReasonStop,
	"MAX_TOKENS": core.StopReasonLength,
	"SAFETY":     core.StopReasonError,
	"RECITATION": core.StopReasonError,
}

func (p *Google) mapFinishReason(reason string) core.StopReason {
	return mapStopReasonFromTable(reason, gemStopReasons)
}
