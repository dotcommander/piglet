package subagent

import (
	"testing"
	"time"
)

func TestPaneStalled(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	limit := 5 * time.Minute

	tests := []struct {
		name         string
		lastActivity time.Time
		want         bool
	}{
		{
			name:         "zero last-activity → false (fresh pane, no output yet)",
			lastActivity: time.Time{},
			want:         false,
		},
		{
			name:         "now - last < limit → false",
			lastActivity: now.Add(-4 * time.Minute),
			want:         false,
		},
		{
			name:         "now - last == limit → false (strictly greater required)",
			lastActivity: now.Add(-limit),
			want:         false,
		},
		{
			name:         "now - last > limit → true",
			lastActivity: now.Add(-5*time.Minute - time.Second),
			want:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := paneStalled(now, tc.lastActivity, limit)
			if got != tc.want {
				t.Errorf("paneStalled(%v, %v, %v) = %v, want %v",
					now, tc.lastActivity, limit, got, tc.want)
			}
		})
	}
}

func TestNormalizePrompt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"Hello World", "hello world"},
		{"  multi   space\ttab ", "multi space tab"},
		{"EXACT match", "exact match"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizePrompt(c.in); got != c.want {
			t.Errorf("normalizePrompt(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDedupCacheHitMiss(t *testing.T) {
	t.Parallel()
	c := &dedupCache{}
	if _, ok := c.lookup("k"); ok {
		t.Fatal("empty cache should miss")
	}
	c.store("k", "result-v1")
	// Stop the ticker started by store to avoid goroutine leak
	t.Cleanup(func() {
		c.mu.Lock()
		if c.ticker != nil {
			c.ticker.Stop()
			close(c.stop)
		}
		c.mu.Unlock()
	})
	got, ok := c.lookup("k")
	if !ok || got != "result-v1" {
		t.Errorf("lookup after store: got (%q, %v), want (result-v1, true)", got, ok)
	}
}

func TestDedupCacheExpiry(t *testing.T) {
	t.Parallel()
	c := &dedupCache{entries: map[string]recentTask{
		"old": {result: "r", completedAt: time.Now().Add(-2 * recentTaskTTL)},
	}}
	if _, ok := c.lookup("old"); ok {
		t.Error("expired entry should not hit")
	}
	// lookup should have pruned the expired entry
	c.mu.Lock()
	_, present := c.entries["old"]
	c.mu.Unlock()
	if present {
		t.Error("expired entry should be removed by lookup")
	}
}

func TestDurationFromMs(t *testing.T) {
	t.Parallel()

	fallback := 10 * time.Minute

	tests := []struct {
		name string
		args map[string]any
		key  string
		want time.Duration
	}{
		{
			name: "nil args → fallback",
			args: nil,
			key:  "timeout_ms",
			want: fallback,
		},
		{
			name: "key missing → fallback",
			args: map[string]any{"other": float64(1000)},
			key:  "timeout_ms",
			want: fallback,
		},
		{
			name: "value is string → fallback",
			args: map[string]any{"timeout_ms": "1500"},
			key:  "timeout_ms",
			want: fallback,
		},
		{
			name: "value is float64(0) → 0 (disabled marker)",
			args: map[string]any{"timeout_ms": float64(0)},
			key:  "timeout_ms",
			want: 0,
		},
		{
			name: "value is float64(-100) → 0 (negative disabled)",
			args: map[string]any{"timeout_ms": float64(-100)},
			key:  "timeout_ms",
			want: 0,
		},
		{
			name: "value is float64(1500) → 1500ms",
			args: map[string]any{"timeout_ms": float64(1500)},
			key:  "timeout_ms",
			want: 1500 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := durationFromMs(tc.args, tc.key, fallback)
			if got != tc.want {
				t.Errorf("durationFromMs(%v, %q, %v) = %v, want %v",
					tc.args, tc.key, fallback, got, tc.want)
			}
		})
	}
}
