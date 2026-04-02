package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// handleEvent processes a single agent event. When batch is true, the caller
// is responsible for returning pollEvents — this function will not include it.
func (m Model) handleEvent(evt core.Event, batch bool) (tea.Model, tea.Cmd) {
	// Dispatch event to registered handlers (event bus)
	if m.cfg.App != nil {
		m.cfg.App.DispatchEvent(m.ctx, evt)
	}

	switch e := evt.(type) {
	case core.EventStreamDelta:
		if e.Kind == "text" {
			m.streamText.WriteString(e.Delta)
			if m.spinnerVerb == "thinking..." {
				m.spinnerVerb = "writing..."
			}
		} else if e.Kind == "thinking" {
			m.streamThink.WriteString(e.Delta)
			if m.spinnerVerb == "thinking..." {
				m.spinnerVerb = "reasoning..."
			}
		}

	case core.EventStreamDone:
		m.streamText.Reset()
		m.streamThink.Reset()

	case core.EventToolStart:
		m.activeTool = e.ToolName
		m.spinnerVerb = "running " + e.ToolName + "..."

	case core.EventToolEnd:
		m.activeTool = ""
		m.spinnerVerb = "thinking..."

		if m.streaming && len(m.inputQueue) > 0 {
			prompts := m.drainPromptQueue()
			if len(prompts) > 0 {
				content := mergePrompts(prompts)
				userMsg := &core.UserMessage{
					Content:   content,
					Timestamp: time.Now(),
				}
				m.appendMessage(userMsg)
				m.followOutput = true
				m.cfg.Agent.Steer(userMsg)
			}
		}

	case core.EventTurnEnd:
		if e.Assistant != nil {
			m.appendMessage(e.Assistant)
			// InputTokens is the full context size (not incremental), so assign rather than accumulate
			m.totalIn = e.Assistant.Usage.InputTokens
			m.totalOut += e.Assistant.Usage.OutputTokens
			m.totalCost += e.Assistant.Usage.Cost
			m.totalCacheRead += e.Assistant.Usage.CacheReadTokens
			m.totalCacheWrite += e.Assistant.Usage.CacheWriteTokens
			m.updateTokenStatus()
			m.status.Set(ext.StatusKeyCost, m.styles.Muted.Render(formatCost(m.totalCost)))
		}
		for _, tr := range e.ToolResults {
			m.appendMessage(tr)
		}

	case core.EventAgentEnd:
		// If background agent is still running, hold back the idle transition
		if m.bgAgent != nil && m.bgAgent.IsRunning() {
			m.heldBackEnd = true
			m.activeTool = ""
			m.spinnerVerb = "waiting for background..."
			m.refreshAndFollow()
			break
		}
		m.stopStreaming()

		return m.drainAndSubmitQueued()

	case core.EventMaxTurns:
		m.messages = append(m.messages, systemNote(fmt.Sprintf("Stopped: max turns reached (%d)", e.Max)))

	case core.EventRetry:
		m.messages = append(m.messages, systemNote(fmt.Sprintf("Retrying (%d/%d): %s", e.Attempt, e.Max, e.Error)))

	case core.EventCompact:
		m.messages = append(m.messages, systemNote(fmt.Sprintf("Context compacted: %d → %d messages", e.Before, e.After)))
		// Rough estimate until the next EventTurnEnd corrects it
		if e.Before > 0 {
			m.totalIn = (m.totalIn * e.After) / e.Before
			m.updateTokenStatus()
		}
		// Persist compacted state so session reload skips original messages
		if m.cfg.Session != nil && m.cfg.Agent != nil {
			_ = m.cfg.Session.AppendCompact(m.cfg.Agent.Messages())
		}
	}

	// Apply actions after event types that may produce them
	var actionCmd tea.Cmd
	switch evt.(type) {
	case core.EventAgentEnd, core.EventTurnEnd, core.EventToolEnd, core.EventCompact:
		actionCmd = m.applyActions()
	}

	// When called from a batch loop, the caller handles pollEvents.
	// For non-batch calls (single eventMsg), return pollEvents here.
	if !batch && m.eventCh != nil && m.streaming {
		if actionCmd != nil {
			return m, tea.Batch(pollEvents(m.eventCh), actionCmd)
		}
		return m, pollEvents(m.eventCh)
	}
	return m, actionCmd
}

// systemNote creates a synthetic assistant message for status/error display.
// These are appended directly to m.messages (not via appendMessage) so they
// are NOT persisted to the session JSONL.
func systemNote(text string) *core.AssistantMessage {
	return &core.AssistantMessage{
		Content: []core.AssistantContent{core.TextContent{Text: text}},
	}
}

func (m *Model) updateTokenStatus() {
	m.status.Set(ext.StatusKeyTokens, m.styles.Muted.Render(formatTokens(m.totalIn, m.totalOut, m.totalCacheRead)))
}

// pollEvents reads the next event from the agent channel, then non-blocking
// drains up to 9 more for batched processing.
func pollEvents(ch <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		// Blocking wait for first event
		first, ok := <-ch
		if !ok {
			return eventsBatchMsg{events: nil}
		}
		events := []core.Event{first}
		// Non-blocking drain of up to 9 more
		for i := 0; i < 9; i++ {
			select {
			case evt, ok := <-ch:
				if !ok {
					return eventsBatchMsg{events: events}
				}
				events = append(events, evt)
			default:
				return eventsBatchMsg{events: events}
			}
		}
		return eventsBatchMsg{events: events}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// drainAndSubmitQueued drains the input queue, executes queued slash commands,
// merges queued prompts into one turn, and starts the agent loop.
// Returns (model, cmd) — cmd is non-nil if a new agent loop was started.
func (m *Model) drainAndSubmitQueued() (Model, tea.Cmd) {
	var batchCmds []tea.Cmd

	if items := m.drainInputQueue(); len(items) > 0 {
		cmds := drainCommands(slices.Clone(items))
		prompts := drainPrompts(items)

		// Execute queued slash commands (collect all, don't early-return)
		for _, c := range cmds {
			name, args := parseSlashCommand(c.content)
			model, cmd := m.runCommand(name, args)
			*m = model.(Model)
			if cmd != nil {
				batchCmds = append(batchCmds, cmd)
			}
		}

		// Merge and submit queued prompts as one turn
		if len(prompts) > 0 {
			content := mergePrompts(prompts)
			userMsg := &core.UserMessage{Content: content, Timestamp: time.Now()}
			m.appendMessage(userMsg)
			m.followOutput = true
			m.refreshAndFollow()
			actionCmd := m.applyActions()
			return *m, tea.Batch(append(batchCmds, actionCmd, m.startAgentLoop(content))...)
		}
	}

	// Apply pending actions even when no prompts were queued (EventAgentEnd
	// early-returns through this function, skipping the post-switch applyActions
	// block in handleEvent — extensions may have queued actions during dispatch).
	if actionCmd := m.applyActions(); actionCmd != nil {
		batchCmds = append(batchCmds, actionCmd)
	}
	if len(batchCmds) > 0 {
		return *m, tea.Batch(batchCmds...)
	}
	return *m, nil
}

func (m Model) handleBgEvent(evt core.Event) (tea.Model, tea.Cmd) {
	switch e := evt.(type) {
	case core.EventStreamDelta:
		if e.Kind == "text" {
			m.bgResult.WriteString(e.Delta)
		}

	case core.EventAgentEnd:
		result := strings.TrimSpace(m.bgResult.String())
		if result == "" {
			result = "(background task produced no output)"
		}
		m.messages = append(m.messages, systemNote(fmt.Sprintf("Background task: %s\n\n%s", m.bgTask, result)))
		m.bgAgent = nil
		m.bgEventCh = nil
		m.bgTask = ""
		m.bgResult.Reset()
		m.status.Set(ext.StatusKeyBg, "")

		// Complete held-back main agent end if needed
		if m.heldBackEnd {
			m.heldBackEnd = false
			m.stopStreaming()

			return m.drainAndSubmitQueued()
		}

		return m, nil
	}

	if m.bgEventCh != nil {
		return m, pollBgEvents(m.bgEventCh)
	}
	return m, nil
}

// pollBgEvents reads the next event from the background agent channel.
func pollBgEvents(ch <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			return bgEventMsg{event: core.EventAgentEnd{}}
		}
		return bgEventMsg{event: evt}
	}
}

// commandNames returns sorted slash command names from the ext.App.
func commandNames(app *ext.App) []string {
	if app == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(app.Commands()))
}

// findModel looks up a model by "provider/name" or plain name.
func findModel(models []core.Model, query string) core.Model {
	for _, m := range models {
		if m.DisplayName() == query || m.Name == query {
			return m
		}
	}
	return core.Model{}
}
