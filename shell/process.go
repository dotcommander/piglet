package shell

import (
	"fmt"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// ProcessEvent handles one agent event. The frontend calls this for each
// event received from the Response.Events channel.
//
// Shell performs: event dispatch, session persistence, action drain,
// queue drain on EventAgentEnd, steering on EventToolEnd.
//
// Returns true if the agent run is complete (EventAgentEnd received and
// no queued input restarted it).
func (s *Shell) ProcessEvent(evt core.Event) (done bool) {
	// Dispatch to registered event handlers
	if s.app != nil {
		s.app.DispatchEvent(s.ctx, evt)
	}

	switch e := evt.(type) {
	case core.EventToolEnd:
		s.handleToolEnd(e)
	case core.EventTurnEnd:
		if e.Assistant != nil {
			s.persistMessage(e.Assistant)
		}
		for _, tr := range e.ToolResults {
			s.persistMessage(tr)
		}
	case core.EventAgentEnd:
		if complete := s.handleAgentEnd(); complete {
			return true
		}
	case core.EventCompact:
		// Persist compacted state, recording the token count before compaction.
		s.mu.Lock()
		sess := s.session
		agent := s.agent
		s.mu.Unlock()
		if sess != nil && agent != nil {
			_ = sess.AppendCompact(agent.Messages(), e.TokensAtCompact)
		}
	}

	// Drain actions for event types that may produce them
	switch evt.(type) {
	case core.EventAgentEnd, core.EventTurnEnd, core.EventToolEnd, core.EventCompact:
		s.drainActions()
	}

	return false
}

// handleToolEnd steers queued prompts into the running agent after a tool call
// completes, allowing mid-turn injection of queued user input.
func (s *Shell) handleToolEnd(_ core.EventToolEnd) {
	s.mu.Lock()
	agent := s.agent
	queue := s.queue
	s.mu.Unlock()

	if agent == nil || len(queue) == 0 {
		return
	}

	prompts := s.drainPromptQueue()
	if len(prompts) == 0 {
		return
	}

	content := mergePrompts(prompts)
	userMsg := &core.UserMessage{
		Content:   content,
		Timestamp: time.Now(),
	}
	s.persistMessage(userMsg)
	s.notify(Notification{Kind: NotifyQueuedSubmit, Text: content})
	agent.Steer(userMsg)
}

// handleAgentEnd processes EventAgentEnd: repairs orphaned tool calls, holds
// back completion if background tasks are still running, otherwise stops the
// agent and drains any queued input. Returns true when the run is fully done.
func (s *Shell) handleAgentEnd() (complete bool) {
	// Repair any orphaned tool calls before anything else.
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()
	if agent != nil {
		msgs := agent.Messages()
		repaired := repairMessageSequence(msgs)
		if len(repaired) != len(msgs) {
			agent.SetMessages(repaired)
		}
	}

	// Check if any bg task is still running
	s.mu.Lock()
	bgRunning := false
	for _, entry := range s.bgTasks {
		if entry.agent != nil && entry.agent.IsRunning() {
			bgRunning = true
			break
		}
	}
	s.mu.Unlock()

	if bgRunning {
		s.mu.Lock()
		s.heldBackEnd = true
		s.mu.Unlock()
		s.drainActions()
		return false
	}

	s.stopRunning()
	s.drainAndSubmitQueued()

	s.mu.Lock()
	restarted := s.running
	s.mu.Unlock()

	return !restarted
}

// ProcessBgEvent handles one background agent event for the given task name.
func (s *Shell) ProcessBgEvent(name string, evt core.Event) (done bool) {
	switch e := evt.(type) {
	case core.EventStreamDelta:
		if e.Kind == "text" {
			s.mu.Lock()
			if entry, ok := s.bgTasks[name]; ok {
				entry.result.WriteString(e.Delta)
			}
			s.mu.Unlock()
		}

	case core.EventAgentEnd:
		s.mu.Lock()
		entry, ok := s.bgTasks[name]
		var result, prompt string
		if ok {
			result = entry.result.String()
			prompt = entry.prompt
			delete(s.bgTasks, name)
		}
		anyRunning := len(s.bgTasks) > 0
		heldBack := s.heldBackEnd
		s.mu.Unlock()

		if result == "" {
			result = "(background task produced no output)"
		}
		s.notify(Notification{
			Kind: NotifyMessage,
			Text: fmt.Sprintf("Background task: %s\n\n%s", prompt, result),
		})
		if !anyRunning {
			s.notify(Notification{Kind: NotifyStatus, Key: ext.StatusKeyBg, Text: ""})
		}

		// Complete held-back main agent end if no bg tasks remain
		if heldBack && !anyRunning {
			s.mu.Lock()
			s.heldBackEnd = false
			s.mu.Unlock()

			s.stopRunning()
			s.drainAndSubmitQueued()

			s.mu.Lock()
			restarted := s.running
			s.mu.Unlock()
			if restarted {
				return false
			}
		}

		return true
	}

	return false
}

// stopRunning transitions from streaming to idle.
func (s *Shell) stopRunning() {
	s.mu.Lock()
	s.running = false
	s.eventCh = nil
	s.mu.Unlock()
	if s.app != nil {
		s.app.SignalIdle()
	}
}
