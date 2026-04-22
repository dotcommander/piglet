// Package safeexec provides filtered-environment subprocess execution with
// inactivity-based timeouts. It is the single place where os/exec is called
// from extensions — all shell helpers route through here.
package safeexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// allowedEnvPrefixes is the set of environment-variable prefixes that are safe
// to forward to subprocesses. Variables not matching any prefix are stripped.
// HOME and PATH are always forwarded unconditionally.
var allowedEnvPrefixes = []string{
	"HOME",
	"PATH",
	"LANG",
	"LC_",
	"TERM",
	"USER",
	"LOGNAME",
	"SHELL",
	"TMPDIR",
	"TMP",
	"TEMP",
	"XDG_",
	"GO",
	"GOPATH",
	"GOROOT",
	"GIT_",
}

// FilterEnv returns a copy of environ containing only safe variables.
// If environ is nil, os.Environ() is used.
func FilterEnv(environ []string) []string {
	if environ == nil {
		environ = os.Environ()
	}
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if envAllowed(kv) {
			out = append(out, kv)
		}
	}
	return out
}

func envAllowed(kv string) bool {
	key, _, _ := strings.Cut(kv, "=")
	for _, prefix := range allowedEnvPrefixes {
		if key == prefix || strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// InactivityTimer cancels a context after the given duration of no Write calls.
// Useful for killing subprocesses that produce no output and appear stalled.
type InactivityTimer struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	timer  *time.Timer
	dur    time.Duration
}

// NewInactivityTimer creates a timer that fires after dur of inactivity.
// The returned cancel func should be deferred by the caller.
func NewInactivityTimer(cancel context.CancelFunc, dur time.Duration) *InactivityTimer {
	t := &InactivityTimer{cancel: cancel, dur: dur}
	if dur > 0 {
		t.timer = time.AfterFunc(dur, cancel)
	}
	return t
}

// Write resets the inactivity countdown. Implements io.Writer for easy wiring
// to cmd.Stdout / cmd.Stderr.
func (t *InactivityTimer) Write(p []byte) (int, error) {
	if t.timer == nil || len(p) == 0 {
		return len(p), nil
	}
	t.mu.Lock()
	t.timer.Reset(t.dur)
	t.mu.Unlock()
	return len(p), nil
}

// Stop cancels the inactivity timer without firing it.
func (t *InactivityTimer) Stop() {
	if t.timer != nil {
		t.timer.Stop()
	}
}

// SafeCmd wraps exec.Cmd with a filtered environment and optional inactivity
// timeout. The caller is responsible for cmd.Start() / cmd.Wait().
//
// Usage:
//
//	cmd, stop := safeexec.SafeCmd(ctx, dir, inactivity, "sh", "-c", command)
//	defer stop()
//	// wire cmd.Stdout / cmd.Stderr as needed
//	err := cmd.Run()
func SafeCmd(ctx context.Context, dir string, inactivity time.Duration, name string, args ...string) (cmd *exec.Cmd, stop func()) {
	runCtx := ctx
	var cancel context.CancelFunc
	if inactivity > 0 {
		runCtx, cancel = context.WithCancel(ctx)
	} else {
		cancel = func() {}
	}

	cmd = exec.CommandContext(runCtx, name, args...)
	cmd.Dir = dir
	cmd.Env = FilterEnv(nil)

	it := NewInactivityTimer(cancel, inactivity)

	// Wrap Stdout/Stderr to reset inactivity timer on each write.
	// Callers that need to capture output should wrap our tee writer.
	cmd.Stdout = it
	cmd.Stderr = it

	stop = func() {
		it.Stop()
		cancel()
	}
	return cmd, stop
}

// Run is a convenience wrapper: executes name+args in dir, captures stdout,
// returns stderr on error. Optionally resets an inactivity watchdog on each
// output chunk.
func Run(ctx context.Context, dir string, inactivity time.Duration, name string, args ...string) (string, error) {
	runCtx := ctx
	var cancel context.CancelFunc
	if inactivity > 0 {
		runCtx, cancel = context.WithCancel(ctx)
	} else {
		cancel = func() {}
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Dir = dir
	cmd.Env = FilterEnv(nil)

	var stdout, stderr bytes.Buffer
	it := NewInactivityTimer(cancel, inactivity)
	defer it.Stop()

	// Tee stdout into both capture buffer and inactivity timer.
	cmd.Stdout = &teeWriter{buf: &stdout, w: it}
	cmd.Stderr = &teeWriter{buf: &stderr, w: it}

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

// teeWriter writes to both buf and w.
type teeWriter struct {
	buf *bytes.Buffer
	w   interface{ Write([]byte) (int, error) }
}

func (t *teeWriter) Write(p []byte) (int, error) {
	n, err := t.buf.Write(p)
	if err != nil {
		return n, err
	}
	_, _ = t.w.Write(p)
	return n, nil
}
