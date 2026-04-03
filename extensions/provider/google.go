package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dotcommander/piglet/core"
	pigletprovider "github.com/dotcommander/piglet/provider"
)

// Google implements core.StreamProvider for Google Generative AI.
type Google struct {
	pigletprovider.BaseProvider
}

// NewGoogle creates a Google provider.
func NewGoogle(model core.Model, apiKeyFn func() string) *Google {
	return &Google{BaseProvider: pigletprovider.NewBaseProvider(model, apiKeyFn)}
}

// Stream implements core.StreamProvider.
func (p *Google) Stream(ctx context.Context, req core.StreamRequest) <-chan core.StreamEvent {
	return pigletprovider.RunStream(ctx, req, p)
}

func (p *Google) StreamModel() core.Model { return p.Model }

// ---------------------------------------------------------------------------
// Request types
// ---------------------------------------------------------------------------

type gemRequest struct {
	Contents         []gemContent  `json:"contents"`
	SystemInstruct   *gemContent   `json:"systemInstruction,omitempty"`
	Tools            []gemTool     `json:"tools,omitempty"`
	GenerationConfig *gemGenConfig `json:"generationConfig,omitempty"`
}

type gemContent struct {
	Role  string    `json:"role"`
	Parts []gemPart `json:"parts"`
}

type gemPart struct {
	Text             string         `json:"text,omitempty"`
	InlineData       *gemInlineData `json:"inlineData,omitempty"`
	FunctionCall     *gemFuncCall   `json:"functionCall,omitempty"`
	FunctionResp     *gemFuncResp   `json:"functionResponse,omitempty"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
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

func (p *Google) BuildRequest(req core.StreamRequest) ([]byte, error) {
	gemReq := gemRequest{
		Contents: p.convertMessages(req),
		GenerationConfig: &gemGenConfig{
			MaxOutputTokens: p.ResolveMaxTokens(req),
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
	return pigletprovider.ConvertMessageList(req.Messages, pigletprovider.MessageConverters[gemContent]{
		User:       p.convertUserMessage,
		Assistant:  p.convertAssistantMessage,
		ToolResult: p.convertToolResult,
	})
}

func (p *Google) convertUserMessage(msg *core.UserMessage) gemContent {
	parts := pigletprovider.DecodeUserBlocks(msg,
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
			part := gemPart{
				FunctionCall: &gemFuncCall{Name: block.Name, Args: block.Arguments},
			}
			if sig, _ := block.ProviderMeta["thoughtSignature"].(string); sig != "" {
				part.ThoughtSignature = sig
			}
			parts = append(parts, part)
		}
	}
	return gemContent{Role: "model", Parts: parts}
}

func (p *Google) convertToolResult(msg *core.ToolResultMessage) gemContent {
	text := pigletprovider.ToolResultText(msg)
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
	decls := pigletprovider.ConvertToolSchemas(tools, func(name, desc string, params any) gemFuncDecl {
		return gemFuncDecl{
			Name:        name,
			Description: desc,
			Parameters:  sanitizeSchemaForGemini(params),
		}
	})
	return []gemTool{{FunctionDeclarations: decls}}
}

// geminiSchemaFields is the set of fields the Gemini API accepts in a Schema
// object. Any field not in this set is stripped before sending.
var geminiSchemaFields = map[string]bool{
	"type": true, "format": true, "title": true, "description": true,
	"nullable": true, "enum": true, "items": true, "properties": true,
	"required": true, "anyOf": true, "default": true, "example": true,
	"pattern": true, "minimum": true, "maximum": true,
	"minItems": true, "maxItems": true,
	"minLength": true, "maxLength": true,
	"minProperties": true, "maxProperties": true,
	"propertyOrdering": true,
}

// sanitizeSchemaForGemini strips fields not supported by the Gemini API.
// It understands JSON Schema structure: "properties" values are keyed by
// user-defined names (not filtered), while schema objects are filtered
// to the allowlist.
func sanitizeSchemaForGemini(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(m))
	for k, v2 := range m {
		if !geminiSchemaFields[k] {
			continue
		}
		switch k {
		case "properties":
			// Keys are user-defined property names — keep all keys,
			// but sanitize each value as a schema object.
			if props, ok := v2.(map[string]any); ok {
				cleaned := make(map[string]any, len(props))
				for name, schema := range props {
					cleaned[name] = sanitizeSchemaForGemini(schema)
				}
				out[k] = cleaned
			} else {
				out[k] = v2
			}
		case "items":
			// items is a schema object.
			out[k] = sanitizeSchemaForGemini(v2)
		case "anyOf":
			// anyOf is an array of schema objects.
			if arr, ok := v2.([]any); ok {
				cleaned := make([]any, len(arr))
				for i, s := range arr {
					cleaned[i] = sanitizeSchemaForGemini(s)
				}
				out[k] = cleaned
			} else {
				out[k] = v2
			}
		default:
			out[k] = v2
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (p *Google) endpoint() string {
	base := strings.TrimSuffix(p.Model.BaseURL, "/")
	return fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", base, p.Model.ID)
}

func (p *Google) SendRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return p.DoHTTPRequest(ctx, p.endpoint(), body, func(req *http.Request) {
		if apiKey := p.APIKeyFn(); apiKey != "" {
			req.Header.Set("x-goog-api-key", apiKey)
		}
	})
}

// ---------------------------------------------------------------------------
// Stream parsing
// ---------------------------------------------------------------------------

type gemResponse struct {
	Candidates    []gemCandidate `json:"candidates"`
	UsageMetadata *gemUsage      `json:"usageMetadata,omitempty"`
}

type gemCandidate struct {
	Content      gemContent `json:"content"`
	FinishReason string     `json:"finishReason"`
}

type gemUsage struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}

func (p *Google) ParseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage {
	var msg core.AssistantMessage
	textBuilders := make(map[int]*strings.Builder)

	pigletprovider.ScanSSE(ctx, reader, ch, func(data []byte) {
		var resp gemResponse
		if err := json.Unmarshal(data, &resp); err != nil {
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
				pigletprovider.AppendTextBuilder(&msg, part.Text, textBuilders)
			}

			if part.FunctionCall != nil {
				tc := core.ToolCall{
					ID:        fmt.Sprintf("call_%d", len(msg.Content)),
					Name:      part.FunctionCall.Name,
					Arguments: part.FunctionCall.Args,
				}
				if part.ThoughtSignature != "" {
					tc.ProviderMeta = map[string]any{
						"thoughtSignature": part.ThoughtSignature,
					}
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
	}, pigletprovider.WithLargeBuffer(256*1024, 10*1024*1024))

	pigletprovider.FinalizeTextBuilders(&msg, textBuilders)

	return msg
}

var gemStopReasons = map[string]core.StopReason{
	"STOP":       core.StopReasonStop,
	"MAX_TOKENS": core.StopReasonLength,
	"SAFETY":     core.StopReasonError,
	"RECITATION": core.StopReasonError,
}

func (p *Google) mapFinishReason(reason string) core.StopReason {
	return pigletprovider.MapStopReasonFromTable(reason, gemStopReasons)
}
