package shell

import "github.com/dotcommander/piglet/core"

// ResponseKind discriminates what Submit produced.
type ResponseKind int

const (
	// ResponseAgentStarted — agent loop started. Consume Response.Events.
	ResponseAgentStarted ResponseKind = iota

	// ResponseQueued — input was queued because agent is already running.
	ResponseQueued

	// ResponseCommand — slash command executed synchronously.
	ResponseCommand

	// ResponseHandled — input transformer consumed the input entirely.
	ResponseHandled

	// ResponseNotReady — agent not yet available (deferred setup).
	ResponseNotReady

	// ResponseError — something failed before the agent could start.
	ResponseError
)

// Response is returned by Submit.
type Response struct {
	Kind   ResponseKind
	Events <-chan core.Event // non-nil only when Kind == ResponseAgentStarted
	Error  error             // non-nil only when Kind == ResponseError
}
