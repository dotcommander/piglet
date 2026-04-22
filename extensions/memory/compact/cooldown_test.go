package compact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeBigRaw builds a raw compact payload with n messages — enough to exceed
// the KeepRecent+1 early-exit check in Handle (default keepRecent=6).
func makeBigRaw(n int) json.RawMessage {
	msgs := make([]WireMsg, n)
	for i := range n {
		data, _ := json.Marshal(map[string]string{
			"role":    "user",
			"content": "some message content that is long enough to matter here yes",
		})
		msgs[i] = WireMsg{Type: "user", Data: data}
	}
	raw, _ := json.Marshal(map[string]any{"messages": msgs})
	return raw
}

// newTestHandler returns a Handler with nil ext/store/reinject and an
// overridden config. Suitable for cooldown unit tests only — it will panic
// if the LLM summarization path is reached, which is fine since the tests
// below all exercise early-exit or cooldown-skip paths.
func newTestHandler(cooldown time.Duration) *Handler {
	cfg := defaultCompactConfig()
	cfg.Cooldown = cooldown
	// Force the sufficient-after-trim early exit so we never reach the LLM path.
	cfg.SufficientAfterTrim = 1_000_000
	return &Handler{cfg: cfg}
}

// TestCooldown_SecondCallSkips verifies that a second Handle call within the
// cooldown window returns the input payload unchanged.
func TestCooldown_SecondCallSkips(t *testing.T) {
	t.Parallel()

	h := newTestHandler(1 * time.Second)

	// Seed lastCompactAt to simulate a compaction that just finished.
	h.lastCompactAt = time.Now()

	raw := makeBigRaw(20)
	out, err := h.Handle(context.Background(), raw)
	require.NoError(t, err)

	// The output must be byte-for-byte identical to the input — no compaction ran.
	assert.Equal(t, string(raw), string(out), "second call within cooldown must return input unchanged")
}

// TestCooldown_AfterIntervalFires verifies that once the cooldown has elapsed,
// Handle proceeds past the guard (i.e., does NOT skip compaction).
// We inject a past lastCompactAt directly to avoid a real sleep.
// "Compaction ran" is detected by lastCompactAt being updated post-call.
func TestCooldown_AfterIntervalFires(t *testing.T) {
	t.Parallel()

	h := newTestHandler(1 * time.Second)

	// Simulate: last compaction happened 2 seconds ago — cooldown has passed.
	past := time.Now().Add(-2 * time.Second)
	h.lastCompactAt = past

	raw := makeBigRaw(20)
	_, err := h.Handle(context.Background(), raw)
	require.NoError(t, err)

	// lastCompactAt must have advanced — the guard did not skip this call.
	h.mu.Lock()
	updated := h.lastCompactAt
	h.mu.Unlock()
	assert.True(t, updated.After(past), "lastCompactAt must advance after a non-skipped compaction")
}

// TestCooldown_Zero_Disabled verifies that Cooldown=0 disables the guard entirely:
// two back-to-back calls both update lastCompactAt (i.e., both run compaction).
func TestCooldown_Zero_Disabled(t *testing.T) {
	t.Parallel()

	h := newTestHandler(0) // no cooldown

	raw := makeBigRaw(20)

	// First call: lastCompactAt is zero — compaction must run.
	before1 := time.Now()
	_, err := h.Handle(context.Background(), raw)
	require.NoError(t, err)
	h.mu.Lock()
	after1 := h.lastCompactAt
	h.mu.Unlock()
	assert.True(t, after1.After(before1), "first call with cooldown=0 must update lastCompactAt")

	// Second call immediately after: cooldown disabled, must also run.
	before2 := time.Now()
	_, err = h.Handle(context.Background(), raw)
	require.NoError(t, err)
	h.mu.Lock()
	after2 := h.lastCompactAt
	h.mu.Unlock()
	assert.True(t, after2.After(before2) || after2.Equal(before2),
		"second call with cooldown=0 must also run compaction (lastCompactAt updated)")
}
