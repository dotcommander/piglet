package safeguard_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dotcommander/piglet/extensions/safeguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolEndJSON serializes an EventToolEnd-shaped payload for handler invocation.
func toolEndJSON(toolName string, isError bool) json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"ToolName": toolName,
		"IsError":  isError,
	})
	return data
}

func TestBreaker_BlocksAfterConsecutiveFailures(t *testing.T) {
	t.Parallel()

	fns := safeguard.NewBreakerFuncs(3, 100*time.Millisecond)
	ctx := context.Background()

	// Simulate 3 consecutive failures via event handler
	for range 3 {
		fns.Handle(ctx, "EventToolEnd", toolEndJSON("broken", true))
	}

	// Before should now block
	allow, _, err := fns.Before(ctx, "broken", nil)
	assert.False(t, allow)
	assert.NoError(t, err)

	// Other tools are unaffected
	allow, _, err = fns.Before(ctx, "healthy", nil)
	assert.True(t, allow)
	assert.NoError(t, err)
}

func TestBreaker_ResetsOnSuccess(t *testing.T) {
	t.Parallel()

	fns := safeguard.NewBreakerFuncs(3, 100*time.Millisecond)
	ctx := context.Background()

	// 2 failures, then success
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("flaky", true))
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("flaky", true))
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("flaky", false))

	// Should still be allowed — counter reset on success
	allow, _, err := fns.Before(ctx, "flaky", nil)
	assert.True(t, allow)
	assert.NoError(t, err)
}

func TestBreaker_CooldownExpiry(t *testing.T) {
	t.Parallel()

	fns := safeguard.NewBreakerFuncs(2, 50*time.Millisecond)
	ctx := context.Background()

	// Trip the breaker
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("slow", true))
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("slow", true))

	allow, _, _ := fns.Before(ctx, "slow", nil)
	require.False(t, allow)

	// Wait for cooldown
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	allow, _, err := fns.Before(ctx, "slow", nil)
	assert.True(t, allow)
	assert.NoError(t, err)
}

func TestBreaker_IgnoresBlockedToolEvents(t *testing.T) {
	t.Parallel()

	fns := safeguard.NewBreakerFuncs(2, 200*time.Millisecond)
	ctx := context.Background()

	// Trip the breaker
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("bad", true))
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("bad", true))

	allow, _, _ := fns.Before(ctx, "bad", nil)
	require.False(t, allow)

	// The blocked call produces EventToolEnd{IsError: false} from the
	// interceptor chain. The handler must not reset the failure state.
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("bad", false))

	// Still blocked
	allow, _, _ = fns.Before(ctx, "bad", nil)
	assert.False(t, allow)
}

func TestBreaker_IndependentPerTool(t *testing.T) {
	t.Parallel()

	fns := safeguard.NewBreakerFuncs(2, time.Second)
	ctx := context.Background()

	// Fail tool A twice
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("a", true))
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("a", true))

	// Fail tool B once
	fns.Handle(ctx, "EventToolEnd", toolEndJSON("b", true))

	// A is blocked, B is not
	allow, _, _ := fns.Before(ctx, "a", nil)
	assert.False(t, allow)

	allow, _, _ = fns.Before(ctx, "b", nil)
	assert.True(t, allow)
}
