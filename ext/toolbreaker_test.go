package ext_test

import (
	"context"
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolCircuitBreaker_BlocksAfterConsecutiveFailures(t *testing.T) {
	t.Parallel()

	ic, eh := ext.NewToolCircuitBreaker(3, 100*time.Millisecond)
	ctx := context.Background()

	// Simulate 3 consecutive failures via event handler
	for range 3 {
		eh.Handle(ctx, core.EventToolEnd{ToolName: "broken", IsError: true})
	}

	// Before should now block
	allow, _, err := ic.Before(ctx, "broken", nil)
	assert.False(t, allow)
	assert.NoError(t, err)

	// Other tools are unaffected
	allow, _, err = ic.Before(ctx, "healthy", nil)
	assert.True(t, allow)
	assert.NoError(t, err)
}

func TestToolCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	t.Parallel()

	ic, eh := ext.NewToolCircuitBreaker(3, 100*time.Millisecond)
	ctx := context.Background()

	// 2 failures, then success
	eh.Handle(ctx, core.EventToolEnd{ToolName: "flaky", IsError: true})
	eh.Handle(ctx, core.EventToolEnd{ToolName: "flaky", IsError: true})
	eh.Handle(ctx, core.EventToolEnd{ToolName: "flaky", IsError: false})

	// Should still be allowed — counter reset on success
	allow, _, err := ic.Before(ctx, "flaky", nil)
	assert.True(t, allow)
	assert.NoError(t, err)
}

func TestToolCircuitBreaker_CooldownExpiry(t *testing.T) {
	t.Parallel()

	ic, eh := ext.NewToolCircuitBreaker(2, 50*time.Millisecond)
	ctx := context.Background()

	// Trip the breaker
	eh.Handle(ctx, core.EventToolEnd{ToolName: "slow", IsError: true})
	eh.Handle(ctx, core.EventToolEnd{ToolName: "slow", IsError: true})

	allow, _, _ := ic.Before(ctx, "slow", nil)
	require.False(t, allow)

	// Wait for cooldown
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	allow, _, err := ic.Before(ctx, "slow", nil)
	assert.True(t, allow)
	assert.NoError(t, err)
}

func TestToolCircuitBreaker_IgnoresBlockedToolEvents(t *testing.T) {
	t.Parallel()

	ic, eh := ext.NewToolCircuitBreaker(2, 200*time.Millisecond)
	ctx := context.Background()

	// Trip the breaker
	eh.Handle(ctx, core.EventToolEnd{ToolName: "bad", IsError: true})
	eh.Handle(ctx, core.EventToolEnd{ToolName: "bad", IsError: true})

	allow, _, _ := ic.Before(ctx, "bad", nil)
	require.False(t, allow)

	// The blocked call produces EventToolEnd{IsError: false} from the
	// interceptor chain. The handler must not reset the failure state.
	eh.Handle(ctx, core.EventToolEnd{ToolName: "bad", IsError: false})

	// Still blocked
	allow, _, _ = ic.Before(ctx, "bad", nil)
	assert.False(t, allow)
}

func TestToolCircuitBreaker_FilterOnlyToolEnd(t *testing.T) {
	t.Parallel()

	_, eh := ext.NewToolCircuitBreaker(3, time.Second)

	assert.True(t, eh.Filter(core.EventToolEnd{}))
	assert.False(t, eh.Filter(core.EventToolStart{}))
	assert.False(t, eh.Filter(core.EventTurnEnd{}))
}

func TestToolCircuitBreaker_IndependentPerTool(t *testing.T) {
	t.Parallel()

	ic, eh := ext.NewToolCircuitBreaker(2, time.Second)
	ctx := context.Background()

	// Fail tool A twice
	eh.Handle(ctx, core.EventToolEnd{ToolName: "a", IsError: true})
	eh.Handle(ctx, core.EventToolEnd{ToolName: "a", IsError: true})

	// Fail tool B once
	eh.Handle(ctx, core.EventToolEnd{ToolName: "b", IsError: true})

	// A is blocked, B is not
	allow, _, _ := ic.Before(ctx, "a", nil)
	assert.False(t, allow)

	allow, _, _ = ic.Before(ctx, "b", nil)
	assert.True(t, allow)
}
