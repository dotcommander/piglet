// Package toolbreaker implements a per-tool circuit breaker for piglet.
// After N consecutive errors, a tool is disabled for the session.
// Reset on success. No persistence — state is session-scoped.
package toolbreaker

import "sync"

// Tracker counts consecutive failures per tool name.
// Zero value is NOT usable — use New().
type Tracker struct {
	mu       sync.Mutex
	failures map[string]int // tool name → consecutive-failure count
}

// New returns an initialized Tracker.
func New() *Tracker {
	return &Tracker{failures: make(map[string]int)}
}

// RecordError increments the consecutive-failure count for toolName.
// Returns the new count.
func (t *Tracker) RecordError(toolName string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[toolName]++
	return t.failures[toolName]
}

// RecordSuccess resets the consecutive-failure count for toolName to zero.
func (t *Tracker) RecordSuccess(toolName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, toolName)
}

// IsDisabled reports whether toolName has reached or exceeded limit consecutive
// failures. limit == 0 means the feature is disabled — always returns false.
func (t *Tracker) IsDisabled(toolName string, limit int) bool {
	if limit <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failures[toolName] >= limit
}

// Reset clears all failure counts. Optional utility for testing and /clear workflows.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures = make(map[string]int)
}

// Count returns the current consecutive-failure count for toolName.
// Exposed for testing; production code uses IsDisabled.
func (t *Tracker) Count(toolName string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failures[toolName]
}
