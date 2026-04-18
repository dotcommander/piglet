package provider

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/dotcommander/piglet/core"
)

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
