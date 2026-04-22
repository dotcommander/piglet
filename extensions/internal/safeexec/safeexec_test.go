package safeexec_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/safeexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []string
		allowed []string
		blocked []string
	}{
		{
			name:    "strips secrets",
			input:   []string{"HOME=/root", "ANTHROPIC_API_KEY=sk-abc", "PATH=/usr/bin", "AWS_SECRET=secret"},
			allowed: []string{"HOME=/root", "PATH=/usr/bin"},
			blocked: []string{"ANTHROPIC_API_KEY=sk-abc", "AWS_SECRET=secret"},
		},
		{
			name:    "allows LANG and LC_ vars",
			input:   []string{"LANG=en_US.UTF-8", "LC_ALL=C", "LC_TIME=de_DE"},
			allowed: []string{"LANG=en_US.UTF-8", "LC_ALL=C", "LC_TIME=de_DE"},
		},
		{
			name:    "allows GIT_ vars",
			input:   []string{"GIT_AUTHOR_NAME=Alice", "GIT_SSH_COMMAND=ssh"},
			allowed: []string{"GIT_AUTHOR_NAME=Alice", "GIT_SSH_COMMAND=ssh"},
		},
		{
			name:    "allows GO vars",
			input:   []string{"GOPATH=/home/user/go", "GOROOT=/usr/local/go", "GOPROXY=direct"},
			allowed: []string{"GOPATH=/home/user/go", "GOROOT=/usr/local/go", "GOPROXY=direct"},
		},
		{
			name:    "blocks unrecognised vars",
			input:   []string{"OPENAI_API_KEY=sk-x", "DATABASE_URL=postgres://", "MY_APP_SECRET=x"},
			blocked: []string{"OPENAI_API_KEY=sk-x", "DATABASE_URL=postgres://", "MY_APP_SECRET=x"},
		},
		{
			name:  "empty input returns empty slice",
			input: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := safeexec.FilterEnv(tc.input)
			gotSet := make(map[string]bool, len(got))
			for _, kv := range got {
				gotSet[kv] = true
			}
			for _, want := range tc.allowed {
				assert.True(t, gotSet[want], "expected %q to be allowed", want)
			}
			for _, nope := range tc.blocked {
				assert.False(t, gotSet[nope], "expected %q to be blocked", nope)
			}
		})
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("captures stdout", func(t *testing.T) {
		t.Parallel()
		out, err := safeexec.Run(context.Background(), t.TempDir(), 0, "sh", "-c", "echo hello")
		require.NoError(t, err)
		assert.Equal(t, "hello\n", out)
	})

	t.Run("returns stderr on failure", func(t *testing.T) {
		t.Parallel()
		_, err := safeexec.Run(context.Background(), t.TempDir(), 0, "sh", "-c", "echo bad >&2; exit 1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // pre-cancel
		_, err := safeexec.Run(ctx, t.TempDir(), 0, "sh", "-c", "sleep 10")
		require.Error(t, err)
	})

	t.Run("inactivity timeout kills stalled process", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		_, err := safeexec.Run(context.Background(), t.TempDir(), 100*time.Millisecond, "sh", "-c", "sleep 10")
		elapsed := time.Since(start)
		require.Error(t, err)
		assert.Less(t, elapsed, 5*time.Second, "should have been killed by inactivity timer")
	})
}

func TestInactivityTimer(t *testing.T) {
	t.Parallel()

	t.Run("fires after inactivity", func(t *testing.T) {
		t.Parallel()
		fired := make(chan struct{}, 1)
		cancel := func() { fired <- struct{}{} }
		timer := safeexec.NewInactivityTimer(cancel, 50*time.Millisecond)
		defer timer.Stop()

		select {
		case <-fired:
			// expected
		case <-time.After(500 * time.Millisecond):
			t.Fatal("inactivity timer did not fire")
		}
	})

	t.Run("write resets the timer", func(t *testing.T) {
		t.Parallel()
		fired := make(chan struct{}, 1)
		cancel := func() { fired <- struct{}{} }
		timer := safeexec.NewInactivityTimer(cancel, 80*time.Millisecond)
		defer timer.Stop()

		// Write repeatedly to keep resetting
		for range 5 {
			time.Sleep(50 * time.Millisecond)
			_, err := timer.Write([]byte("ping"))
			require.NoError(t, err)
		}

		select {
		case <-fired:
			t.Fatal("timer fired while we were still writing")
		default:
		}

		// Now stop writing and let it fire
		select {
		case <-fired:
			// expected
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timer did not fire after writes stopped")
		}
	})

	t.Run("stop prevents firing", func(t *testing.T) {
		t.Parallel()
		fired := false
		cancel := func() { fired = true }
		timer := safeexec.NewInactivityTimer(cancel, 50*time.Millisecond)
		timer.Stop()
		time.Sleep(150 * time.Millisecond)
		assert.False(t, fired, "timer should not fire after Stop")
	})
}

func TestSafeCmd(t *testing.T) {
	t.Parallel()

	t.Run("env is filtered", func(t *testing.T) {
		t.Parallel()
		cmd, stop := safeexec.SafeCmd(context.Background(), t.TempDir(), 0, "sh", "-c", "env")
		defer stop()

		var out strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &out
		require.NoError(t, cmd.Run())

		// Must not contain any API keys from the current environment
		for _, line := range strings.Split(out.String(), "\n") {
			key, _, _ := strings.Cut(line, "=")
			assert.False(t,
				strings.Contains(strings.ToUpper(key), "SECRET") ||
					strings.Contains(strings.ToUpper(key), "API_KEY") ||
					strings.Contains(strings.ToUpper(key), "TOKEN"),
				"leaked env var: %q", line)
		}
	})
}
