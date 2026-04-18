package safeguard

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// breakerState is the shared state between the interceptor (Before gate)
// and the event handler (failure tracking). Both reference the same instance.
type breakerState struct {
	mu       sync.Mutex
	failures map[string]int       // tool name → consecutive failure count
	disabled map[string]time.Time // tool name → cooldown expiry
	maxFails int
	cooldown time.Duration
}

func newBreakerState(maxFails int, cooldown time.Duration) *breakerState {
	return &breakerState{
		failures: make(map[string]int),
		disabled: make(map[string]time.Time),
		maxFails: maxFails,
		cooldown: cooldown,
	}
}

func (s *breakerState) isDisabled(toolName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.disabled[toolName]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(s.disabled, toolName)
		delete(s.failures, toolName)
		return false
	}
	return true
}

func (s *breakerState) record(toolName string, isError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Skip tracking for tools currently in cooldown — their EventToolEnd
	// comes from the interceptor block, not from actual execution.
	if _, ok := s.disabled[toolName]; ok {
		return
	}

	if !isError {
		delete(s.failures, toolName)
		return
	}
	s.failures[toolName]++
	if s.failures[toolName] >= s.maxFails {
		s.disabled[toolName] = time.Now().Add(s.cooldown)
	}
}

// BreakerFuncs holds the raw function closures for the circuit breaker,
// used directly in tests to verify breaker logic without SDK machinery.
type BreakerFuncs struct {
	Before  func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error)
	Preview func(ctx context.Context, toolName string, args map[string]any) string
	Handle  func(ctx context.Context, eventType string, data json.RawMessage) *sdk.Action
}

// NewBreakerFuncs creates the circuit breaker function closures.
// maxFails is the number of consecutive errors before a tool is disabled.
// cooldown is the duration after which the breaker auto-resets.
//
// Used by RegisterBreaker and directly by tests.
func NewBreakerFuncs(maxFails int, cooldown time.Duration) BreakerFuncs {
	state := newBreakerState(maxFails, cooldown)

	before := func(_ context.Context, toolName string, _ map[string]any) (bool, map[string]any, error) {
		if state.isDisabled(toolName) {
			// Return allow=false with nil error. The interceptor chain
			// returns a ToolResult (not an error), so EventToolEnd.IsError
			// will be false — no feedback loop.
			return false, nil, nil
		}
		return true, nil, nil
	}

	preview := func(_ context.Context, toolName string, _ map[string]any) string {
		return fmt.Sprintf("tool %q is temporarily disabled (circuit breaker open)", toolName)
	}

	handle := func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
		var evt struct {
			ToolName string `json:"ToolName"`
			IsError  bool   `json:"IsError"`
		}
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil
		}
		state.record(evt.ToolName, evt.IsError)
		return nil
	}

	return BreakerFuncs{Before: before, Preview: preview, Handle: handle}
}

// RegisterBreaker adds the tool circuit breaker interceptor and event handler to e.
// It disables a tool after maxFails consecutive errors; re-enables after cooldown elapses.
//
// The interceptor Before hook blocks calls to disabled tools, returning a synthetic
// "tool temporarily disabled" result. The event handler tracks consecutive failures
// from EventToolEnd.IsError.
func RegisterBreaker(e *sdk.Extension, maxFails int, cooldown time.Duration) {
	fns := NewBreakerFuncs(maxFails, cooldown)

	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "tool-circuit-breaker",
		Priority: 1000,
		Before:   fns.Before,
		Preview:  fns.Preview,
	})

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:   "tool-circuit-breaker",
		Events: []string{"EventToolEnd"},
		Handle: fns.Handle,
	})
}
