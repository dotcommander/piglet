package ext

import (
	"context"
	"sync"
	"time"

	"github.com/dotcommander/piglet/core"
)

// toolBreakerState is the shared state between the interceptor (Before gate)
// and the event handler (failure tracking). Both reference the same instance.
type toolBreakerState struct {
	mu       sync.Mutex
	failures map[string]int       // tool name → consecutive failure count
	disabled map[string]time.Time // tool name → cooldown expiry
	maxFails int
	cooldown time.Duration
}

func (s *toolBreakerState) isDisabled(toolName string) bool {
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

func (s *toolBreakerState) record(toolName string, isError bool) {
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

// NewToolCircuitBreaker creates an interceptor and event handler pair that
// disables tools after maxFails consecutive errors. The tool is re-enabled
// after cooldown elapses.
//
// The interceptor Before hook blocks calls to disabled tools. The event handler
// tracks consecutive failures from EventToolEnd.IsError.
//
// Register both: app.RegisterInterceptor(ic) and app.RegisterEventHandler(eh).
func NewToolCircuitBreaker(maxFails int, cooldown time.Duration) (Interceptor, EventHandler) {
	state := &toolBreakerState{
		failures: make(map[string]int),
		disabled: make(map[string]time.Time),
		maxFails: maxFails,
		cooldown: cooldown,
	}

	ic := Interceptor{
		Name:     "tool-circuit-breaker",
		Priority: 1000,
		Before: func(_ context.Context, toolName string, _ map[string]any) (bool, map[string]any, error) {
			if state.isDisabled(toolName) {
				// Return allow=false with nil error. The interceptor chain
				// returns a ToolResult (not an error), so EventToolEnd.IsError
				// will be false — no feedback loop.
				return false, nil, nil
			}
			return true, nil, nil
		},
	}

	eh := EventHandler{
		Name:     "tool-circuit-breaker",
		Priority: 0,
		Filter: func(evt core.Event) bool {
			_, ok := evt.(core.EventToolEnd)
			return ok
		},
		Handle: func(_ context.Context, evt core.Event) Action {
			te := evt.(core.EventToolEnd)
			state.record(te.ToolName, te.IsError)
			return nil
		},
	}

	return ic, eh
}
