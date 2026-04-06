package shell

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/session"
)

// Config holds everything Shell needs to start.
type Config struct {
	App      *ext.App
	Agent    *core.Agent      // may be nil (deferred setup)
	Session  *session.Session // may be nil (persistence disabled)
	Settings *config.Settings
}

// Shell manages the agent lifecycle on behalf of any frontend.
// It is a concrete struct — adding methods is non-breaking for all consumers.
type Shell struct {
	app      *ext.App
	agent    *core.Agent
	session  *session.Session
	settings *config.Settings
	ctx      context.Context

	mu       sync.Mutex
	queue    []queuedItem
	running  bool
	eventCh  <-chan core.Event
	quitting bool

	// Background agent
	bgAgent   *core.Agent
	bgEventCh <-chan core.Event
	bgTask    string
	bgResult  strings.Builder

	// Main agent ended but bg agent still running
	heldBackEnd bool

	// Accumulated notifications for the frontend
	notifications []Notification
}

// New creates a Shell. The agent may be nil and set later via SetAgent.
func New(ctx context.Context, cfg Config) *Shell {
	return &Shell{
		app:      cfg.App,
		agent:    cfg.Agent,
		session:  cfg.Session,
		settings: cfg.Settings,
		ctx:      ctx,
	}
}

// SetAgent wires the agent after deferred setup completes.
// Calls ext.App.Bind internally with background agent callbacks.
func (s *Shell) SetAgent(agent *core.Agent, opts ...ext.BindOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agent = agent

	allOpts := append(s.bgBindOpts(), opts...)
	s.app.Bind(agent, allOpts...)
}

// bgBindOpts returns the standard background-agent bind options.
func (s *Shell) bgBindOpts() []ext.BindOption {
	return []ext.BindOption{
		ext.WithRunBackground(s.startBackground),
		ext.WithCancelBackground(s.StopBackground),
		ext.WithIsBackgroundRunning(s.isBackgroundRunning),
		ext.WithAbortWithMarker(s.AbortWithMarker),
		ext.WithSteer(s.Steer),
	}
}

// Agent returns the current agent, or nil if not yet set.
func (s *Shell) Agent() *core.Agent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agent
}

// Session returns the current session, or nil.
func (s *Shell) Session() *session.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session
}

// App returns the ext.App.
func (s *Shell) App() *ext.App { return s.app }

// IsRunning returns true if an agent loop is currently active.
func (s *Shell) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Abort cancels the current agent run without blocking. The agent
// goroutine finishes asynchronously and emits EventAgentEnd, which
// the frontend uses to transition out of the streaming state.
// No marker is inserted — the LLM does not see the interruption.
func (s *Shell) Abort() {
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()
	if agent != nil {
		agent.Cancel()
	}
}

// AbortWithMarker cancels the current agent run and persists a marker
// message to the session so the LLM sees the interruption context on
// the next run. Use for programmatic cancellations (e.g. plan-mode
// transitions) where the model needs to know the prior run was interrupted.
func (s *Shell) AbortWithMarker(reason string) {
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()
	if agent == nil {
		return
	}
	agent.Cancel()
	marker := &core.UserMessage{
		Content:   "[Request interrupted: " + reason + "]",
		Timestamp: time.Now(),
	}
	s.persistMessage(marker)
}

// Steer injects a steering message into the running agent turn.
// Returns the disposition: delivered (active run), queued (idle), or dropped (no agent).
func (s *Shell) Steer(content string) ext.SteerDisposition {
	s.mu.Lock()
	agent := s.agent
	running := s.running
	s.mu.Unlock()

	if agent == nil {
		return ext.SteerDropped
	}
	if running {
		agent.Steer(&core.UserMessage{Content: content})
		return ext.SteerDelivered
	}
	// Agent exists but not running — queue for next run.
	s.enqueue(content, false)
	return ext.SteerQueued
}

// Commands returns all registered slash commands.
func (s *Shell) Commands() map[string]*ext.Command {
	if s.app == nil {
		return nil
	}
	return s.app.Commands()
}

// CommandNames returns sorted slash command names.
func (s *Shell) CommandNames() []string {
	if s.app == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(s.app.Commands()))
}

// Messages returns the conversation history snapshot.
func (s *Shell) Messages() []core.Message {
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()
	if agent != nil {
		return agent.Messages()
	}
	return nil
}

// QueueSize returns the number of pending queued inputs.
func (s *Shell) QueueSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// Quitting returns true if a quit action was processed.
func (s *Shell) Quitting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quitting
}

// EventChannel returns the current main agent event channel, or nil.
// Used by frontends to detect when ProcessEvent restarted the agent.
func (s *Shell) EventChannel() <-chan core.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eventCh
}

// BgEventChannel returns the background agent event channel, or nil.
func (s *Shell) BgEventChannel() <-chan core.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bgEventCh
}

// Notifications returns and clears all pending notifications.
// Call after each ProcessEvent or Submit.
func (s *Shell) Notifications() []Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.notifications
	s.notifications = nil
	return out
}

// notify appends a notification. Must be called with s.mu held or from
// a method that has exclusive access.
func (s *Shell) notify(n Notification) {
	s.notifications = append(s.notifications, n)
}

// SetSession swaps the active session. Used when ActionSwapSession is
// handled externally (by a frontend that needs to update display state).
func (s *Shell) SetSession(sess *session.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = sess
}

// persistMessage writes a message to the session JSONL.
func (s *Shell) persistMessage(msg core.Message) {
	if s.session != nil {
		if err := s.session.Append(msg); err != nil {
			slog.Warn("session: failed to persist message", "error", err)
		}
	}
}

// EnqueueResult re-enqueues an action result from an async execution.
// Used by frontends that run ActionRunAsync outside ProcessEvent.
func (s *Shell) EnqueueResult(action ext.Action) {
	if s.app != nil && action != nil {
		s.app.EnqueueAction(action)
	}
}
