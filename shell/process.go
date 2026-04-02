package shell

import (
	"fmt"
	"slices"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/session"
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
		// Steer queued prompts mid-turn
		s.mu.Lock()
		agent := s.agent
		queue := s.queue
		s.mu.Unlock()

		if agent != nil && len(queue) > 0 {
			prompts := s.drainPromptQueue()
			if len(prompts) > 0 {
				content := mergePrompts(prompts)
				userMsg := &core.UserMessage{
					Content:   content,
					Timestamp: time.Now(),
				}
				s.persistMessage(userMsg)
				agent.Steer(userMsg)
			}
		}
		_ = e // used for type switch

	case core.EventTurnEnd:
		if e.Assistant != nil {
			s.persistMessage(e.Assistant)
		}
		for _, tr := range e.ToolResults {
			s.persistMessage(tr)
		}

	case core.EventAgentEnd:
		// Check if bg agent is still running
		s.mu.Lock()
		bgRunning := s.bgAgent != nil && s.bgAgent.IsRunning()
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

		if restarted {
			return false // agent was restarted with queued input
		}
		return true

	case core.EventCompact:
		// Persist compacted state
		s.mu.Lock()
		sess := s.session
		agent := s.agent
		s.mu.Unlock()
		if sess != nil && agent != nil {
			_ = sess.AppendCompact(agent.Messages())
		}
	}

	// Drain actions for event types that may produce them
	switch evt.(type) {
	case core.EventAgentEnd, core.EventTurnEnd, core.EventToolEnd, core.EventCompact:
		s.drainActions()
	}

	return false
}

// ProcessBgEvent handles one background agent event.
func (s *Shell) ProcessBgEvent(evt core.Event) (done bool) {
	switch e := evt.(type) {
	case core.EventStreamDelta:
		if e.Kind == "text" {
			s.mu.Lock()
			s.bgResult.WriteString(e.Delta)
			s.mu.Unlock()
		}

	case core.EventAgentEnd:
		s.mu.Lock()
		result := s.bgResult.String()
		task := s.bgTask
		heldBack := s.heldBackEnd
		s.bgAgent = nil
		s.bgEventCh = nil
		s.bgTask = ""
		s.bgResult.Reset()
		s.mu.Unlock()

		if result == "" {
			result = "(background task produced no output)"
		}
		s.notify(Notification{
			Kind: NotifyMessage,
			Text: fmt.Sprintf("Background task: %s\n\n%s", task, result),
		})
		s.notify(Notification{Kind: NotifyStatus, Key: ext.StatusKeyBg, Text: ""})

		// Complete held-back main agent end
		if heldBack {
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

// DrainActions processes pending ext.App actions. Public for cases where
// frontends trigger actions outside the normal event flow (shortcuts, modal
// callbacks, async action results).
func (s *Shell) DrainActions() { s.drainActions() }

// drainActions processes pending ext.App actions, classifying each as
// internal (handled by Shell) or frontend-visible (surfaced as Notification).
func (s *Shell) drainActions() {
	if s.app == nil {
		return
	}

	for _, action := range s.app.PendingActions() {
		switch act := action.(type) {
		// --- Shell handles internally ---
		case ext.ActionSetSessionTitle:
			s.mu.Lock()
			sess := s.session
			s.mu.Unlock()
			if sess != nil && act.Title != "" {
				_ = sess.SetTitle(act.Title)
			}
			s.notify(Notification{Kind: NotifySessionTitle, Text: act.Title, Action: act})

		case ext.ActionRunAsync:
			// Run synchronously — frontends that need async can check Action
			// and handle it themselves via EnqueueResult.
			result := act.Fn()
			if result != nil {
				s.app.EnqueueAction(result)
			}

		case ext.ActionSendMessage:
			s.mu.Lock()
			running := s.running
			s.mu.Unlock()
			if running {
				s.enqueue(act.Content, false)
			} else {
				s.notify(Notification{Kind: NotifySendMessage, Text: act.Content, Action: act})
			}

		case ext.ActionSwapSession:
			if sess, ok := act.Session.(*session.Session); ok {
				s.mu.Lock()
				old := s.session
				s.session = sess
				agent := s.agent
				s.mu.Unlock()
				if old != nil && old != sess {
					_ = old.Close()
				}
				if agent != nil {
					agent.SetMessages(sess.Messages())
				}
				s.notify(Notification{Kind: NotifySessionSwap, Action: act})
			}

		case ext.ActionQuit:
			s.StopBackground()
			s.mu.Lock()
			s.quitting = true
			s.mu.Unlock()
			s.notify(Notification{Kind: NotifyQuit})

		// --- Surfaced to frontend ---
		case ext.ActionShowMessage:
			s.notify(Notification{Kind: NotifyMessage, Text: act.Text, Action: act})

		case ext.ActionNotify:
			kind := NotifyMessage
			switch act.Level {
			case "warn":
				kind = NotifyWarn
			case "error":
				kind = NotifyError
			}
			s.notify(Notification{Kind: kind, Text: act.Message, Action: act})

		case ext.ActionSetStatus:
			s.notify(Notification{Kind: NotifyStatus, Key: act.Key, Text: act.Text, Action: act})

		case ext.ActionShowPicker:
			s.notify(Notification{Kind: NotifyPicker, Action: act})

		case ext.ActionAttachImage:
			s.notify(Notification{Kind: NotifyImage, Action: act})

		case ext.ActionDetachImage:
			s.notify(Notification{Kind: NotifyImage, Action: act})

		case ext.ActionSetWidget:
			s.notify(Notification{Kind: NotifyWidget, Key: act.Key, Action: act})

		case ext.ActionShowOverlay:
			s.notify(Notification{Kind: NotifyOverlay, Key: act.Key, Action: act})

		case ext.ActionCloseOverlay:
			s.notify(Notification{Kind: NotifyOverlay, Key: act.Key, Action: act})

		case ext.ActionExec:
			s.notify(Notification{Kind: NotifyExec, Action: act})
		}
	}
}

// drainAndSubmitQueued drains the input queue, executes queued slash commands,
// merges queued prompts into one turn, and restarts the agent if needed.
func (s *Shell) drainAndSubmitQueued() {
	s.mu.Lock()
	items := drainQueue(&s.queue)
	s.mu.Unlock()

	if len(items) == 0 {
		// Still drain actions — EventAgentEnd handlers may have queued some
		s.drainActions()
		return
	}

	cmds := drainCommands(slices.Clone(items))
	prompts := drainPrompts(items)

	// Execute queued slash commands
	for _, c := range cmds {
		name, args := parseSlashCommand(c.content)
		s.runCommand(name, args)
	}

	// Merge and submit queued prompts as one turn
	if len(prompts) > 0 {
		content := mergePrompts(prompts)
		userMsg := &core.UserMessage{Content: content, Timestamp: time.Now()}
		s.persistMessage(userMsg)
		s.startAgent(content)
	}

	s.drainActions()
}

// drainPromptQueue returns only non-command items from the queue (mid-turn steering).
func (s *Shell) drainPromptQueue() []queuedItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	var prompts []queuedItem
	j := 0
	for _, it := range s.queue {
		if it.priority == priorityLater {
			s.queue[j] = it
			j++
		} else {
			prompts = append(prompts, it)
		}
	}
	s.queue = s.queue[:j]
	return prompts
}
