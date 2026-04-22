package tui

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/errfmt"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/shell"
)

// handleEvent processes a single agent event. When batch is true, the caller
// is responsible for returning pollEvents — this function will not include it.
func (m Model) handleEvent(evt core.Event, batch bool) (tea.Model, tea.Cmd) {
	// --- TUI-specific rendering updates (Shell does NOT handle these) ---
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
		summary := shell.ToolSummary(e.ToolName, e.Args)
		m.activeTool = summary
		m.spinnerVerb = "running " + summary + "..."

	case core.EventToolEnd:
		m.activeTool = ""
		m.spinnerVerb = "thinking..."

	case core.EventTurnEnd:
		if e.Assistant != nil {
			m.appendDisplayMessage(e.Assistant)
			m.totalIn = e.Assistant.Usage.InputTokens
			m.totalOut += e.Assistant.Usage.OutputTokens
			m.totalCost += e.Assistant.Usage.Cost
			m.totalCacheRead += e.Assistant.Usage.CacheReadTokens
			m.totalCacheWrite += e.Assistant.Usage.CacheWriteTokens
			m.updateTokenStatus()
			m.status.Set(ext.StatusKeyCost, m.styles.Muted.Render(formatCost(m.totalCost)))
		}
		for _, tr := range e.ToolResults {
			m.appendDisplayMessage(tr)
		}

	case core.EventAgentEnd:
		// Check if shell held back the end (bg agent still running)
		if m.shell != nil && m.shell.IsRunning() {
			// Shell decided to hold back — update UI to show waiting state
			m.activeTool = ""
			m.spinnerVerb = "waiting for background..."
			m.refreshAndFollow()
		} else {
			m.stopStreaming()
		}

	case core.EventMaxTurns:
		m.messages = append(m.messages, systemNote(fmt.Sprintf("Stopped: max turns reached (%d)", e.Max)))

	case core.EventRetry:
		// e.Error is a string — wrap for classification so errfmt can augment retry errors with hints.
		m.messages = append(m.messages, systemNote(fmt.Sprintf("Retrying (%d/%d): %s", e.Attempt, e.Max, errfmt.FormatForDisplay(errors.New(e.Error)))))

	case core.EventCompact:
		m.messages = append(m.messages, systemNote(fmt.Sprintf("Context compacted: %d → %d messages", e.Before, e.After)))
		if e.Before > 0 {
			m.totalIn = (m.totalIn * e.After) / e.Before
			m.updateTokenStatus()
		}
	}

	// --- Delegate to Shell for persistence, event dispatch, action drain ---
	var done bool
	if m.shell != nil {
		done = m.shell.ProcessEvent(evt)
	}

	// Apply shell notifications to TUI state
	var actionCmd tea.Cmd
	switch evt.(type) {
	case core.EventAgentEnd, core.EventTurnEnd, core.EventToolEnd, core.EventCompact:
		actionCmd = m.applyShellNotifications()
	}

	// Check if shell restarted the agent with queued input
	if m.shell != nil {
		if ch := m.shell.EventChannel(); ch != nil && ch != m.eventCh {
			m.eventCh = ch
			m.streaming = true
			m.spinnerVerb = "thinking..."
			// Will be polled by the batch or non-batch logic below
		}
	}

	if done {
		m.stopStreaming()
	}

	if !batch && m.eventCh != nil && m.streaming {
		if actionCmd != nil {
			return m, tea.Batch(pollEvents(m.eventCh), actionCmd)
		}
		return m, pollEvents(m.eventCh)
	}
	return m, actionCmd
}

// systemNote creates a synthetic assistant message for status/error display.
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
		first, ok := <-ch
		if !ok {
			return eventsBatchMsg{events: nil}
		}
		events := make([]core.Event, 1, 10)
		events[0] = first
		for range 9 {
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

func (m Model) handleBgEvent(evt core.Event) (tea.Model, tea.Cmd) {
	if m.shell == nil {
		return m, nil
	}

	done := m.shell.ProcessBgEvent(m.bgTaskName, evt)
	notifyCmd := m.applyShellNotifications()

	if done {
		m.bgEventCh = nil
		m.bgTaskName = ""
		// Check if shell restarted the main agent (held-back end completed)
		if ch := m.shell.EventChannel(); ch != nil && ch != m.eventCh {
			m.eventCh = ch
			m.streaming = true
			m.spinnerVerb = "thinking..."
			return m, tea.Batch(pollEvents(ch), tickCmd(), m.spinner.Tick, notifyCmd)
		}
		if !m.shell.IsRunning() {
			m.stopStreaming()
		}
		return m, notifyCmd
	}

	if m.bgEventCh != nil {
		return m, tea.Batch(pollBgEvents(m.bgEventCh), notifyCmd)
	}
	return m, notifyCmd
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

// commandSuggestions returns sorted command suggestions (name + description) from the ext.App.
func commandSuggestions(app *ext.App) []CommandSuggestion {
	if app == nil {
		return nil
	}
	cmds := app.Commands()
	names := slices.Sorted(maps.Keys(cmds))
	out := make([]CommandSuggestion, 0, len(names))
	for _, name := range names {
		out = append(out, CommandSuggestion{Name: name, Description: cmds[name].Description})
	}
	return out
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
