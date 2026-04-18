package provider

import (
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
