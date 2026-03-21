package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dotcommander/piglet/core"
)

// baseProvider holds fields shared by all provider implementations.
type baseProvider struct {
	model      core.Model
	apiKeyFn   func() string
	httpClient *http.Client
}

func newBaseProvider(model core.Model, apiKeyFn func() string) baseProvider {
	return baseProvider{
		model:    model,
		apiKeyFn: apiKeyFn,
		httpClient: &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 100,
			},
		},
	}
}

// doHTTPRequest handles the shared HTTP POST + status check logic.
func (b *baseProvider) doHTTPRequest(ctx context.Context, url string, body []byte, setHeaders func(*http.Request)) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if setHeaders != nil {
		setHeaders(req)
	}

	for k, v := range b.model.Headers {
		req.Header.Set(k, v)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s API error %d: %s", b.model.Provider, resp.StatusCode, string(errBody))
	}

	return resp.Body, nil
}

// streamPipeline is implemented by each concrete provider to plug into runStream.
type streamPipeline interface {
	buildRequest(req core.StreamRequest) ([]byte, error)
	sendRequest(ctx context.Context, body []byte) (io.ReadCloser, error)
	parseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage
	streamModel() core.Model
}

// runStream is the shared Stream() goroutine template.
func runStream(ctx context.Context, req core.StreamRequest, p streamPipeline) <-chan core.StreamEvent {
	ch := make(chan core.StreamEvent, 32)

	go func() {
		defer close(ch)

		body, err := p.buildRequest(req)
		if err != nil {
			ch <- core.StreamEvent{Type: core.StreamError, Error: fmt.Errorf("build request: %w", err)}
			return
		}

		reader, err := p.sendRequest(ctx, body)
		if err != nil {
			ch <- core.StreamEvent{Type: core.StreamError, Error: err}
			return
		}
		defer reader.Close()

		msg := p.parseResponse(ctx, reader, ch)
		m := p.streamModel()
		if msg.StopReason == "" {
			msg.StopReason = core.StopReasonStop
		}
		msg.Model = m.ID
		msg.Provider = m.Provider
		msg.Timestamp = time.Now()

		ch <- core.StreamEvent{Type: core.StreamDone, Message: &msg}
	}()

	return ch
}
