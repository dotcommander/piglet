package core

import (
	"context"
	"sync"
	"time"
)

// Agent buffer and concurrency constants.
const (
	EventBufferSize    = 100
	EmitTimeout        = 250 * time.Millisecond
	ToolConcurrency    = 10
	MaxRetryAttempts   = 3
	RetryBaseDelay     = 500 * time.Millisecond
	RetryMaxDelay      = 5 * time.Second
)

// StepAction is the user's response when step mode pauses before a tool.
type StepAction int

const (
	StepApprove StepAction = iota
	StepSkip
	StepAbort
)

// AgentConfig configures the agent loop.
type AgentConfig struct {
	System   string
	Model    Model
	Provider StreamProvider
	Tools    []Tool
	Options  StreamOptions

	MaxTurns int // 0 = unlimited

	// OnCompact is called when token usage exceeds CompactAt.
	// It receives the current messages and should return a summary string.
	// If nil, compaction is disabled.
	OnCompact func(messages []Message) (string, error)
	CompactAt int // token threshold; 0 = disabled
}

// Agent manages the agent loop: streaming, tool execution, steering, events.
type Agent struct {
	cfg AgentConfig
	mu  sync.RWMutex

	messages []Message
	events   chan Event
	cancel   context.CancelFunc
	done     chan struct{}
	running  bool

	// Queues
	steerMu  sync.Mutex
	steerQ   []Message
	followQ  []Message

	// Step mode
	stepMode bool
	stepGate chan StepAction

	// Reusable timer for emit() to avoid per-call time.After leaks.
	emitTimer *time.Timer
}

// NewAgent creates an agent with the given configuration.
func NewAgent(cfg AgentConfig) *Agent {
	t := time.NewTimer(EmitTimeout)
	// Drain so the timer is in the "stopped and ready to reset" state.
	if !t.Stop() {
		<-t.C
	}
	return &Agent{
		cfg:       cfg,
		messages:  make([]Message, 0, 64),
		events:    make(chan Event, EventBufferSize),
		done:      make(chan struct{}),
		emitTimer: t,
	}
}

// Start begins the agent loop with the given user prompt.
// Returns a channel of events. The channel is closed when the agent finishes.
func (a *Agent) Start(ctx context.Context, prompt string) <-chan Event {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return a.events
	}
	a.running = true
	ctx, a.cancel = context.WithCancel(ctx)
	a.done = make(chan struct{})
	a.events = make(chan Event, EventBufferSize)
	a.mu.Unlock()

	go a.run(ctx, prompt)
	return a.events
}

// Stop cancels the agent and waits for it to finish.
func (a *Agent) Stop() {
	a.mu.RLock()
	cancel := a.cancel
	done := a.done
	a.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Steer injects a message that interrupts the current turn.
// Remaining tool calls are cancelled and the message is processed next.
func (a *Agent) Steer(msg Message) {
	a.steerMu.Lock()
	a.steerQ = append(a.steerQ, msg)
	a.steerMu.Unlock()
}

// FollowUp queues a message for after the agent finishes its current run.
func (a *Agent) FollowUp(msg Message) {
	a.steerMu.Lock()
	a.followQ = append(a.followQ, msg)
	a.steerMu.Unlock()
}

// Messages returns a snapshot of the conversation history.
func (a *Agent) Messages() []Message {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]Message, len(a.messages))
	copy(out, a.messages)
	return out
}

// AppendMessage adds a message to the conversation history.
func (a *Agent) AppendMessage(msg Message) {
	a.mu.Lock()
	a.messages = append(a.messages, msg)
	a.mu.Unlock()
}

// SetMessages replaces the conversation history (used after compaction).
func (a *Agent) SetMessages(msgs []Message) {
	a.mu.Lock()
	a.messages = msgs
	a.mu.Unlock()
}

// SetTools updates the active tool set.
func (a *Agent) SetTools(tools []Tool) {
	a.mu.Lock()
	a.cfg.Tools = tools
	a.mu.Unlock()
}

// SetModel updates the model for future LLM calls.
func (a *Agent) SetModel(m Model) {
	a.mu.Lock()
	a.cfg.Model = m
	a.mu.Unlock()
}

// SetSystem updates the system prompt.
func (a *Agent) SetSystem(s string) {
	a.mu.Lock()
	a.cfg.System = s
	a.mu.Unlock()
}

// SetStepMode enables or disables step-by-step tool approval.
func (a *Agent) SetStepMode(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stepMode = on
	if on {
		a.stepGate = make(chan StepAction, 1)
	} else {
		a.stepGate = nil
	}
}

// StepMode returns whether step mode is enabled.
func (a *Agent) StepMode() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.stepMode
}

// StepRespond sends an approval action to the waiting tool.
func (a *Agent) StepRespond(action StepAction) {
	a.mu.RLock()
	gate := a.stepGate
	a.mu.RUnlock()
	if gate == nil {
		return
	}
	// Drain stale value
	select {
	case <-gate:
	default:
	}
	gate <- action
}

// IsRunning returns whether the agent loop is active.
func (a *Agent) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

// ---------------------------------------------------------------------------
// Internal: main loop
// ---------------------------------------------------------------------------

func (a *Agent) run(ctx context.Context, prompt string) {
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		close(a.done)
		close(a.events)
	}()

	a.emit(EventAgentStart{})

	// Add the user prompt as a message
	a.mu.Lock()
	a.messages = append(a.messages, &UserMessage{
		Content:   prompt,
		Timestamp: time.Now(),
	})
	a.mu.Unlock()

	// Outer loop: handles follow-ups
	for {
		stopped := a.runTurns(ctx)
		if stopped {
			break
		}

		follow := a.dequeueFollow()
		if len(follow) == 0 {
			break
		}

		// Add follow-up messages and continue
		a.mu.Lock()
		for _, m := range follow {
			a.messages = append(a.messages, m)
		}
		a.mu.Unlock()
	}

	a.emit(EventAgentEnd{Messages: a.Messages()})
}

// runTurns executes the inner turn loop. Returns true if the agent should stop.
func (a *Agent) runTurns(ctx context.Context) bool {
	turnCount := 0
	pending := a.dequeueSteer()

	for {
		if ctx.Err() != nil {
			return true
		}

		a.emit(EventTurnStart{})

		// Add any pending steering messages to history
		if len(pending) > 0 {
			a.mu.Lock()
			for _, m := range pending {
				a.messages = append(a.messages, m)
			}
			a.mu.Unlock()
		}

		// Stream assistant response with retry
		assistant, err := a.streamWithRetry(ctx)
		if err != nil {
			return true
		}

		// Add assistant message to history
		a.mu.Lock()
		a.messages = append(a.messages, assistant)
		a.mu.Unlock()

		if assistant.StopReason == StopReasonError || assistant.StopReason == StopReasonAborted {
			a.emit(EventTurnEnd{Assistant: assistant})
			return true
		}

		// Extract tool calls
		toolCalls := extractToolCalls(assistant)

		// Execute tools in parallel
		var toolResults []*ToolResultMessage
		var steeringFromTools []Message
		if len(toolCalls) > 0 {
			toolResults, steeringFromTools = a.executeTools(ctx, toolCalls)

			// Add tool results to history
			a.mu.Lock()
			for _, tr := range toolResults {
				a.messages = append(a.messages, tr)
			}
			a.mu.Unlock()
		}

		a.emit(EventTurnEnd{Assistant: assistant, ToolResults: toolResults})

		// Check max turns
		turnCount++
		if a.cfg.MaxTurns > 0 && turnCount >= a.cfg.MaxTurns {
			a.emit(EventMaxTurns{Count: turnCount, Max: a.cfg.MaxTurns})
			return true
		}

		// Determine next pending messages
		if len(steeringFromTools) > 0 {
			pending = steeringFromTools
		} else {
			pending = a.dequeueSteer()
		}

		// If no tool calls and no pending, we're done
		if len(toolCalls) == 0 && len(pending) == 0 {
			return false
		}
	}
}

// ---------------------------------------------------------------------------
// Internal: helpers
// ---------------------------------------------------------------------------

func (a *Agent) emit(evt Event) {
	if !a.emitTimer.Stop() {
		select {
		case <-a.emitTimer.C:
		default:
		}
	}
	a.emitTimer.Reset(EmitTimeout)
	select {
	case a.events <- evt:
	case <-a.emitTimer.C:
		// Drop event if consumer isn't keeping up
	}
}

func (a *Agent) dequeueSteer() []Message {
	a.steerMu.Lock()
	defer a.steerMu.Unlock()
	if len(a.steerQ) == 0 {
		return nil
	}
	msgs := a.steerQ
	a.steerQ = nil
	return msgs
}

func (a *Agent) dequeueFollow() []Message {
	a.steerMu.Lock()
	defer a.steerMu.Unlock()
	if len(a.followQ) == 0 {
		return nil
	}
	msgs := a.followQ
	a.followQ = nil
	return msgs
}
