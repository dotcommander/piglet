package sdk

import "context"

// ToggleStepMode toggles the agent's step-by-step mode and returns the new state.
func (e *Extension) ToggleStepMode(ctx context.Context) (bool, error) {
	type resp struct {
		On bool `json:"on"`
	}
	r, err := hostCall[resp](e, ctx, "host/toggleStepMode", struct{}{})
	if err != nil {
		return false, err
	}
	return r.On, nil
}

// RequestQuit asks the host to quit the TUI (fire-and-forget).
func (e *Extension) RequestQuit(ctx context.Context) error {
	return hostCallVoid(e, ctx, "host/requestQuit", struct{}{})
}

// Abort cancels the agent's current run silently and waits for acknowledgement.
// No [Request interrupted] marker is inserted. Use for programmatic cancellation
// where the LLM should not see a user-interruption artifact on resume.
// For fire-and-forget cancellation, use the notification-based Abort() on the
// Extension's notify surface instead.
func (e *Extension) AbortSync(ctx context.Context) error {
	return hostCallVoid(e, ctx, "host/abort", struct{}{})
}

// HasCompactor returns true if a compactor is currently registered with the host.
func (e *Extension) HasCompactor(ctx context.Context) (bool, error) {
	type resp struct {
		Present bool `json:"present"`
	}
	r, err := hostCall[resp](e, ctx, "host/hasCompactor", struct{}{})
	if err != nil {
		return false, err
	}
	return r.Present, nil
}

// TriggerCompactResult holds the message counts before and after compaction.
type TriggerCompactResult struct {
	Before int `json:"before"`
	After  int `json:"after"`
}

// TriggerCompact asks the host to run compaction immediately using the registered
// compactor. Returns (before, after) message counts on success, or an error if
// no compactor is registered or compaction fails.
func (e *Extension) TriggerCompact(ctx context.Context) (TriggerCompactResult, error) {
	return hostCall[TriggerCompactResult](e, ctx, "host/triggerCompact", struct{}{})
}
