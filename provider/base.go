package provider

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dotcommander/piglet/core"
)

const debugResponseCap = 512 * 1024 // 512KB

type limitedWriter struct {
	buf bytes.Buffer
	cap int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.buf.Len() >= w.cap {
		return len(p), nil
	}
	if rem := w.cap - w.buf.Len(); len(p) > rem {
		w.buf.Write(p[:rem])
		return len(p), nil
	}
	return w.buf.Write(p)
}

func (w *limitedWriter) Truncated() bool { return w.buf.Len() >= w.cap }

// BaseProvider holds fields shared by all provider implementations.
type BaseProvider struct {
	Model      core.Model
	APIKeyFn   func() string
	HTTPClient *http.Client
	Logger     *slog.Logger // nil = no debug logging
}

func NewBaseProvider(model core.Model, apiKeyFn func() string) BaseProvider {
	return BaseProvider{
		Model:    model,
		APIKeyFn: apiKeyFn,
		HTTPClient: &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 100,
			},
		},
	}
}

func (b *BaseProvider) SetLogger(l *slog.Logger) {
	b.Logger = l
}

func (b *BaseProvider) DebugLog() *slog.Logger {
	return b.Logger
}

func (b *BaseProvider) ResolveMaxTokens(req core.StreamRequest) int {
	if req.Options.MaxTokens != nil {
		return *req.Options.MaxTokens
	}
	return b.Model.MaxTokens
}

// DoHTTPRequest handles the shared HTTP POST + status check logic.
func (b *BaseProvider) DoHTTPRequest(ctx context.Context, url string, body []byte, setHeaders func(*http.Request)) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if setHeaders != nil {
		setHeaders(req)
	}

	for k, v := range b.Model.Headers {
		req.Header.Set(k, v)
	}

	resp, err := b.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		err := fmt.Errorf("%s API error %d: %s", b.Model.Provider, resp.StatusCode, string(errBody))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			if IsLoopbackURL(b.Model.BaseURL) {
				return nil, fmt.Errorf("%s: local server requires authentication. Check your server config.", resp.Status)
			}
			return nil, fmt.Errorf("%w\n\nSet your API key: export %s=<key>\nOr add to ~/.config/piglet/auth.json", err, envKeyForProvider(b.Model.Provider))
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			if d := parseRetryAfter(resp.Header.Get("Retry-After")); d > 0 {
				return nil, &RetryAfterError{Err: err, Duration: d}
			}
		}
		return nil, err
	}

	return resp.Body, nil
}

func envKeyForProvider(provider string) string {
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "google":
		return "GOOGLE_API_KEY"
	case "xai":
		return "XAI_API_KEY"
	case "groq":
		return "GROQ_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	default:
		return strings.ToUpper(provider) + "_API_KEY"
	}
}

// StreamPipeline is implemented by each concrete provider to plug into RunStream.
type StreamPipeline interface {
	BuildRequest(req core.StreamRequest) ([]byte, error)
	SendRequest(ctx context.Context, body []byte) (io.ReadCloser, error)
	ParseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage
	StreamModel() core.Model
}

type Debuggable interface {
	SetLogger(l *slog.Logger)
}

type DebugLogger interface {
	DebugLog() *slog.Logger
}

// ConvertToolSchemas iterates tool schemas, normalises nil parameters,
// and calls build to produce provider-specific tool definitions.
func ConvertToolSchemas[T any](tools []core.ToolSchema, build func(name, desc string, params any) T) []T {
	out := make([]T, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = map[string]any{"type": "object"}
		}
		out = append(out, build(t.Name, t.Description, params))
	}
	return out
}

// MessageConverters holds per-type callbacks for converting core messages
// to a provider-specific wire type.
type MessageConverters[T any] struct {
	User       func(*core.UserMessage) T
	Assistant  func(*core.AssistantMessage) T
	ToolResult func(*core.ToolResultMessage) T
}

// ConvertMessageList applies the appropriate converter for each message type.
func ConvertMessageList[T any](msgs []core.Message, conv MessageConverters[T]) []T {
	var out []T
	for _, m := range msgs {
		switch msg := m.(type) {
		case *core.UserMessage:
			out = append(out, conv.User(msg))
		case *core.AssistantMessage:
			out = append(out, conv.Assistant(msg))
		case *core.ToolResultMessage:
			out = append(out, conv.ToolResult(msg))
		}
	}
	return out
}

// MapStopReasonFromTable looks up a provider-specific stop reason string
// in the given table, returning core.StopReasonStop as default.
func MapStopReasonFromTable(reason string, table map[string]core.StopReason) core.StopReason {
	if r, ok := table[reason]; ok {
		return r
	}
	return core.StopReasonStop
}

var (
	sseDataSpace  = []byte("data: ")
	sseData       = []byte("data:")
	sseDone       = []byte("[DONE]")
	sseOpenBrace  = []byte("{")
	sseCloseBrace = []byte("}")
)

// extractSSEData extracts the data payload from an SSE line.
func extractSSEData(line []byte) []byte {
	trimmed := bytes.TrimSpace(line)
	if after, ok := bytes.CutPrefix(trimmed, sseDataSpace); ok {
		return after
	}
	if after, ok := bytes.CutPrefix(trimmed, sseData); ok {
		return after
	}
	// Some providers send raw JSON
	if bytes.HasPrefix(trimmed, sseOpenBrace) && bytes.HasSuffix(trimmed, sseCloseBrace) {
		return trimmed
	}
	return nil
}

// ToolResultText extracts joined text from a ToolResultMessage.
func ToolResultText(msg *core.ToolResultMessage) string {
	var parts []string
	for _, b := range msg.Content {
		if tc, ok := b.(core.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// AppendTextBuilder accumulates delta into a strings.Builder keyed by content index.
// If no TextContent exists in msg, one is appended.
func AppendTextBuilder(msg *core.AssistantMessage, delta string, builders map[int]*strings.Builder) {
	for i := range msg.Content {
		if _, ok := msg.Content[i].(core.TextContent); ok {
			b, exists := builders[i]
			if !exists {
				b = &strings.Builder{}
				builders[i] = b
			}
			b.WriteString(delta)
			return
		}
	}
	idx := len(msg.Content)
	msg.Content = append(msg.Content, core.TextContent{})
	b := &strings.Builder{}
	b.WriteString(delta)
	builders[idx] = b
}

// FinalizeTextBuilders writes accumulated text from builders into msg.Content.
func FinalizeTextBuilders(msg *core.AssistantMessage, builders map[int]*strings.Builder) {
	for idx, b := range builders {
		if idx < len(msg.Content) {
			msg.Content[idx] = core.TextContent{Text: b.String()}
		}
	}
}

// DecodeUserBlocks converts a UserMessage's Content and Blocks into
// provider-specific typed slices using the supplied callbacks.
func DecodeUserBlocks[T any](msg *core.UserMessage, text func(string) T, image func(core.ImageContent) T) []T {
	var out []T
	if msg.Content != "" {
		out = append(out, text(msg.Content))
	}
	for _, b := range msg.Blocks {
		switch c := b.(type) {
		case core.TextContent:
			out = append(out, text(c.Text))
		case core.ImageContent:
			out = append(out, image(c))
		}
	}
	return out
}

// ScanSSE reads SSE lines from reader, calling handler for each non-empty
// data payload. Respects context cancellation (sends StreamError on cancel).
func ScanSSE(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent, handler func(data []byte), opts ...ScanOption) {
	scanner := bufio.NewScanner(reader)
	for _, opt := range opts {
		opt(scanner)
	}
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- core.StreamEvent{Type: core.StreamError, Error: ctx.Err()}
			return
		default:
		}
		data := extractSSEData(scanner.Bytes())
		if len(data) == 0 || bytes.Equal(data, sseDone) {
			continue
		}
		handler(data)
	}
}

type ScanOption func(*bufio.Scanner)

func WithLargeBuffer(initial, max int) ScanOption {
	return func(s *bufio.Scanner) {
		s.Buffer(make([]byte, 0, initial), max)
	}
}

func ComputeCost(u core.Usage, c core.ModelCost) float64 {
	return (float64(u.InputTokens)*c.Input +
		float64(u.OutputTokens)*c.Output +
		float64(u.CacheReadTokens)*c.CacheRead +
		float64(u.CacheWriteTokens)*c.CacheWrite) / 1_000_000
}

// RunStream is the shared Stream() goroutine template.
func RunStream(ctx context.Context, req core.StreamRequest, p StreamPipeline) <-chan core.StreamEvent {
	ch := make(chan core.StreamEvent, 32)

	go func() {
		defer close(ch)

		body, err := p.BuildRequest(req)
		if err != nil {
			ch <- core.StreamEvent{Type: core.StreamError, Error: fmt.Errorf("build request: %w", err)}
			return
		}

		// Debug: log request payload
		var logger *slog.Logger
		if dl, ok := p.(DebugLogger); ok {
			logger = dl.DebugLog()
		}
		if logger != nil {
			logger.Debug("request",
				"provider", p.StreamModel().Provider,
				"model", p.StreamModel().ID,
				"body", string(body),
			)
		}

		reader, err := p.SendRequest(ctx, body)
		if err != nil {
			ch <- core.StreamEvent{Type: core.StreamError, Error: err}
			return
		}
		defer reader.Close()

		// Debug: tee response stream to logger (capped at debugResponseCap)
		var parseReader io.Reader = reader
		if logger != nil {
			lw := &limitedWriter{cap: debugResponseCap}
			parseReader = io.TeeReader(reader, lw)
			defer func() {
				logger.Debug("response",
					"provider", p.StreamModel().Provider,
					"model", p.StreamModel().ID,
					"body", lw.buf.String(),
					"truncated", lw.Truncated(),
				)
			}()
		}

		msg := p.ParseResponse(ctx, parseReader, ch)
		m := p.StreamModel()
		if msg.StopReason == "" {
			msg.StopReason = core.StopReasonStop
		}
		msg.Model = m.ID
		msg.Provider = m.Provider
		msg.Timestamp = time.Now()
		msg.Usage.Cost = ComputeCost(msg.Usage, m.Cost)

		ch <- core.StreamEvent{Type: core.StreamDone, Message: &msg}
	}()

	return ch
}
