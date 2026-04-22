// Package errfmt classifies errors and produces user-facing display text with
// actionable hints. It is the single seam between raw provider/network errors
// and the UI — callers pass any error, errfmt decides whether to augment it.
//
// Classification order: most-specific first (local-server auth before generic
// auth, rate-limit before server-error). Unknown errors return nil from
// HintsForError and fall through to raw err.Error() in FormatForDisplay.
package errfmt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/piglet/provider"
)

// AuthDiagnosticCode is a stable string identifier for authentication error
// subtypes. Extensions may inspect this programmatically.
type AuthDiagnosticCode string

const (
	AuthMissingAPIKey AuthDiagnosticCode = "auth.missing_api_key"
	AuthLocalServer   AuthDiagnosticCode = "auth.local_server"
	AuthInvalidKey    AuthDiagnosticCode = "auth.invalid_key"
)

// ErrorHint describes what went wrong and what the user can do about it.
type ErrorHint struct {
	Summary  string             // one-line description of what went wrong
	Hints    []string           // actionable steps, each a complete sentence or command
	DiagCode AuthDiagnosticCode // empty for non-auth errors
}

// HintsForError classifies err and returns a hint. Returns nil for nil errors
// and for errors that don't match any known pattern.
func HintsForError(err error) *ErrorHint {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	// Local-server auth — check before generic 401/403 so the message
	// "401 Unauthorized: local server requires authentication" routes here
	// even when the numeric status is present, and also catches cases where
	// the caller omits the status code.
	case strings.Contains(msg, "local server") || strings.Contains(msg, "loopback"):
		return localServerHint()

	// Auth errors (non-local).
	case strings.Contains(msg, "401") || strings.Contains(msg, "403"):
		return missingAPIKeyHint(msg)

	// Rate limit.
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		return rateLimitHint(err)

	// Network errors.
	case strings.Contains(msg, "connection refused"):
		return connectionRefusedHint()
	case strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "timed out"):
		return timeoutHint()

	// Server errors.
	case strings.Contains(msg, "500") || strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") || strings.Contains(msg, "504"):
		return serverErrorHint()

	default:
		return nil
	}
}

// FormatForDisplay returns a display-ready string. If no hint is found it
// returns err.Error() unchanged. Otherwise it renders Summary followed by
// each hint as a bullet.
func FormatForDisplay(err error) string {
	h := HintsForError(err)
	if h == nil {
		return err.Error()
	}
	var sb strings.Builder
	sb.WriteString(h.Summary)
	for _, hint := range h.Hints {
		sb.WriteString("\n  • ")
		sb.WriteString(hint)
	}
	return sb.String()
}

// ---- private hint constructors ----

func missingAPIKeyHint(msg string) *ErrorHint {
	envVar := inferEnvVar(msg)
	hints := []string{
		"Or add to ~/.config/piglet/auth.json",
	}
	if envVar != "" {
		hints = append([]string{fmt.Sprintf("Run: export %s=<your-key>", envVar)}, hints...)
	} else {
		hints = append([]string{"Set the API key environment variable for your provider"}, hints...)
	}
	return &ErrorHint{
		Summary:  "API key missing or invalid",
		Hints:    hints,
		DiagCode: AuthMissingAPIKey,
	}
}

func localServerHint() *ErrorHint {
	return &ErrorHint{
		Summary: "Local server requires authentication",
		Hints: []string{
			"Check your server configuration",
			"Verify the server is started correctly",
		},
		DiagCode: AuthLocalServer,
	}
}

func rateLimitHint(err error) *ErrorHint {
	hints := []string{
		"Switch to a different model with /model",
		"Reduce message frequency",
	}
	// Prepend a duration-specific hint when Retry-After header was present.
	var rae *provider.RetryAfterError
	if errors.As(err, &rae) && rae.Duration > 0 {
		secs := int(rae.Duration.Round(time.Second).Seconds())
		hints = append([]string{fmt.Sprintf("Wait %d seconds and retry", secs)}, hints...)
	}
	return &ErrorHint{
		Summary: "Rate limit reached",
		Hints:   hints,
	}
}

func connectionRefusedHint() *ErrorHint {
	return &ErrorHint{
		Summary: "Connection refused",
		Hints: []string{
			"Is the server running?",
			"Check the base URL in ~/.config/piglet/config.yaml",
		},
	}
}

func timeoutHint() *ErrorHint {
	return &ErrorHint{
		Summary: "Request timed out",
		Hints: []string{
			"Check network connectivity",
			"The provider may be slow — retry in a moment",
		},
	}
}

func serverErrorHint() *ErrorHint {
	return &ErrorHint{
		Summary: "Provider server error",
		Hints: []string{
			"The provider may be experiencing issues",
			"Retry in a few minutes",
		},
	}
}

// inferEnvVar extracts the likely API key environment variable name from the
// error string by scanning for a known provider name. Falls back to "".
func inferEnvVar(msg string) string {
	providers := []struct {
		name   string
		envVar string
	}{
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
		{"google", "GOOGLE_API_KEY"},
		{"openrouter", "OPENROUTER_API_KEY"},
		{"groq", "GROQ_API_KEY"},
		{"xai", "XAI_API_KEY"},
	}
	lower := strings.ToLower(msg)
	for _, p := range providers {
		if strings.Contains(lower, p.name) {
			return p.envVar
		}
	}
	return ""
}
