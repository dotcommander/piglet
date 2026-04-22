package errfmt_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/piglet/errfmt"
	"github.com/dotcommander/piglet/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHintsForError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantNil     bool
		wantCode    errfmt.AuthDiagnosticCode
		wantSummary string
		wantHint    string // substring expected in at least one Hint
	}{
		{
			name:    "nil error",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "unknown error",
			err:     errors.New("something completely random"),
			wantNil: true,
		},
		{
			name:        "401 openai auth",
			err:         errors.New("openai API error 401: Unauthorized"),
			wantCode:    errfmt.AuthMissingAPIKey,
			wantSummary: "API key missing or invalid",
			wantHint:    "OPENAI_API_KEY",
		},
		{
			name:        "403 anthropic auth",
			err:         errors.New("anthropic API error 403: Forbidden"),
			wantCode:    errfmt.AuthMissingAPIKey,
			wantSummary: "API key missing or invalid",
			wantHint:    "ANTHROPIC_API_KEY",
		},
		{
			name:        "401 unknown provider",
			err:         errors.New("some-provider API error 401: Unauthorized"),
			wantCode:    errfmt.AuthMissingAPIKey,
			wantSummary: "API key missing or invalid",
		},
		{
			name:        "local server auth",
			err:         errors.New("local server requires authentication. Check your server config."),
			wantCode:    errfmt.AuthLocalServer,
			wantSummary: "Local server requires authentication",
		},
		{
			name:        "loopback server auth",
			err:         errors.New("loopback: 401 Unauthorized"),
			wantCode:    errfmt.AuthLocalServer,
			wantSummary: "Local server requires authentication",
		},
		{
			name:        "429 rate limit",
			err:         errors.New("openai API error 429: Too Many Requests"),
			wantSummary: "Rate limit reached",
			wantHint:    "model",
		},
		{
			name: "429 with retry-after duration",
			err: &provider.RetryAfterError{
				Err:      errors.New("openai API error 429: rate limit exceeded"),
				Duration: 30 * time.Second,
			},
			wantSummary: "Rate limit reached",
			wantHint:    "30 seconds",
		},
		{
			name:        "rate limit string",
			err:         errors.New("rate limit exceeded, slow down"),
			wantSummary: "Rate limit reached",
		},
		{
			name:        "connection refused",
			err:         errors.New("dial tcp 127.0.0.1:11434: connection refused"),
			wantSummary: "Connection refused",
			wantHint:    "server running",
		},
		{
			name:        "context deadline exceeded",
			err:         errors.New("context deadline exceeded"),
			wantSummary: "Request timed out",
			wantHint:    "connectivity",
		},
		{
			name:        "i/o timeout",
			err:         errors.New("read tcp: i/o timeout"),
			wantSummary: "Request timed out",
		},
		{
			name:        "timed out",
			err:         errors.New("request timed out after 300s"),
			wantSummary: "Request timed out",
		},
		{
			name:        "500 server error",
			err:         errors.New("openai API error 500: Internal Server Error"),
			wantSummary: "Provider server error",
			wantHint:    "Retry",
		},
		{
			name:        "502 bad gateway",
			err:         errors.New("openai API error 502: Bad Gateway"),
			wantSummary: "Provider server error",
		},
		{
			name:        "503 service unavailable",
			err:         errors.New("openai API error 503: Service Unavailable"),
			wantSummary: "Provider server error",
		},
		{
			name:        "504 gateway timeout",
			err:         errors.New("openai API error 504: Gateway Timeout"),
			wantSummary: "Provider server error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := errfmt.HintsForError(tc.err)

			if tc.wantNil {
				assert.Nil(t, h)
				return
			}

			require.NotNil(t, h)
			assert.Equal(t, tc.wantSummary, h.Summary)

			if tc.wantCode != "" {
				assert.Equal(t, tc.wantCode, h.DiagCode)
			}

			if tc.wantHint != "" {
				found := false
				for _, hint := range h.Hints {
					if strings.Contains(hint, tc.wantHint) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected hint containing %q, got: %v", tc.wantHint, h.Hints)
			}

			assert.NotEmpty(t, h.Hints, "hints slice must not be empty")
		})
	}
}

func TestFormatForDisplay(t *testing.T) {
	t.Parallel()

	t.Run("nil hint falls through to raw error", func(t *testing.T) {
		t.Parallel()
		err := errors.New("something completely random")
		got := errfmt.FormatForDisplay(err)
		assert.Equal(t, err.Error(), got)
	})

	t.Run("known error renders summary and bullets", func(t *testing.T) {
		t.Parallel()
		err := errors.New("openai API error 401: Unauthorized")
		got := errfmt.FormatForDisplay(err)
		assert.Contains(t, got, "API key missing or invalid")
		assert.Contains(t, got, "\n  • ")
		assert.Contains(t, got, "OPENAI_API_KEY")
	})

	t.Run("rate limit with duration includes seconds", func(t *testing.T) {
		t.Parallel()
		err := &provider.RetryAfterError{
			Err:      errors.New("openai API error 429: rate limited"),
			Duration: 60 * time.Second,
		}
		got := errfmt.FormatForDisplay(err)
		assert.Contains(t, got, "Rate limit reached")
		assert.Contains(t, got, "60 seconds")
	})

	t.Run("connection refused renders bullets", func(t *testing.T) {
		t.Parallel()
		err := errors.New("connection refused")
		got := errfmt.FormatForDisplay(err)
		assert.Contains(t, got, "Connection refused")
		assert.Contains(t, got, "\n  • ")
	})
}
