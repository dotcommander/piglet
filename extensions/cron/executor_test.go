package cron

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Executor tests cover Execute's dispatch and the shell/webhook actions.
// The "prompt" action is intentionally untested here — it invokes a `piglet`
// binary on PATH which may or may not exist on the test machine; the dispatch
// itself is exercised via the shell/webhook/unknown paths.

func TestExecuteShellSuccess(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell action assumes POSIX sh")
	}
	res := Execute(context.Background(), "demo", TaskConfig{
		Action:  "shell",
		Command: "printf hello",
	})
	assert.True(t, res.Success)
	assert.Equal(t, "hello", res.Output)
	assert.Empty(t, res.Error)
	assert.GreaterOrEqual(t, res.DurationMs, int64(0))
}

func TestExecuteShellFailureCapturesOutput(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell action assumes POSIX sh")
	}
	res := Execute(context.Background(), "demo", TaskConfig{
		Action:  "shell",
		Command: "echo oops 1>&2; exit 3",
	})
	assert.False(t, res.Success)
	assert.Contains(t, res.Output, "oops")
	assert.NotEmpty(t, res.Error)
}

func TestExecuteShellMissingCommand(t *testing.T) {
	t.Parallel()
	res := Execute(context.Background(), "demo", TaskConfig{Action: "shell"})
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "shell action requires command")
}

func TestExecuteUnknownAction(t *testing.T) {
	t.Parallel()
	res := Execute(context.Background(), "demo", TaskConfig{Action: "wat"})
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, `unknown action "wat"`)
}

func TestExecuteWebhookSuccess(t *testing.T) {
	t.Parallel()
	var seenMethod, seenHeader, seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenHeader = r.Header.Get("X-Test")
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		seenBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	res := Execute(context.Background(), "demo", TaskConfig{
		Action:  "webhook",
		URL:     srv.URL,
		Method:  http.MethodPut,
		Body:    "hello",
		Headers: map[string]string{"X-Test": "v"},
	})
	assert.True(t, res.Success)
	assert.Contains(t, res.Output, "HTTP 200")
	assert.Equal(t, http.MethodPut, seenMethod)
	assert.Equal(t, "v", seenHeader)
	assert.Equal(t, "hello", seenBody)
}

func TestExecuteWebhookDefaultsToPOST(t *testing.T) {
	t.Parallel()
	var seenMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	res := Execute(context.Background(), "demo", TaskConfig{Action: "webhook", URL: srv.URL})
	assert.True(t, res.Success)
	assert.Equal(t, http.MethodPost, seenMethod)
}

func TestExecuteWebhookHTTPErrorIsFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	res := Execute(context.Background(), "demo", TaskConfig{Action: "webhook", URL: srv.URL})
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "HTTP 500")
}

func TestExecuteWebhookMissingURL(t *testing.T) {
	t.Parallel()
	res := Execute(context.Background(), "demo", TaskConfig{Action: "webhook"})
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "webhook action requires url")
}
