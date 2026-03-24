package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

func (m Model) handleEvent(evt core.Event) (tea.Model, tea.Cmd) {
	// Dispatch event to registered handlers (event bus)
	if m.cfg.App != nil {
		m.cfg.App.DispatchEvent(context.Background(), evt)
	}

	switch e := evt.(type) {
	case core.EventStreamDelta:
		if e.Kind == "text" {
			m.streamText += e.Delta
			if m.spinnerVerb == "thinking..." {
				m.spinnerVerb = "writing..."
			}
		} else if e.Kind == "thinking" {
			m.streamThink += e.Delta
			if m.spinnerVerb == "thinking..." {
				m.spinnerVerb = "reasoning..."
			}
		}

	case core.EventStreamDone:
		m.streamText = ""
		m.streamThink = ""

	case core.EventToolStart:
		m.activeTool = e.ToolName
		m.spinnerVerb = "running " + e.ToolName + "..."

	case core.EventToolEnd:
		m.activeTool = ""
		m.spinnerVerb = "thinking..."

	case core.EventTurnEnd:
		if e.Assistant != nil {
			m.messages = append(m.messages, e.Assistant)
			// InputTokens is the full context size (not incremental), so assign rather than accumulate
			m.totalIn = e.Assistant.Usage.InputTokens
			m.totalOut += e.Assistant.Usage.OutputTokens
			m.totalCost += e.Assistant.Usage.Cost
			m.totalCacheRead += e.Assistant.Usage.CacheReadTokens
			m.totalCacheWrite += e.Assistant.Usage.CacheWriteTokens
			m.updateTokenStatus()
			m.status.Set(ext.StatusKeyCost, m.styles.Muted.Render(formatCost(m.totalCost)))

			if m.cfg.Session != nil {
				_ = m.cfg.Session.Append(e.Assistant)
			}
		}
		for _, tr := range e.ToolResults {
			m.messages = append(m.messages, tr)
			if m.cfg.Session != nil {
				_ = m.cfg.Session.Append(tr)
			}
		}

	case core.EventAgentEnd:
		m.streaming = false
		m.activeTool = ""
		m.spinnerVerb = ""
		m.status.SetSpinnerView("")
		m.streamCache = StreamCache{}
		m.refreshViewport()
		if m.followOutput {
			m.viewport.GotoBottom()
		}

	case core.EventMaxTurns:
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{
				core.TextContent{Text: fmt.Sprintf("Stopped: max turns reached (%d)", e.Max)},
			},
		})

	case core.EventRetry:
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{
				core.TextContent{Text: fmt.Sprintf("Retrying (%d/%d): %s", e.Attempt, e.Max, e.Error)},
			},
		})

	case core.EventCompact:
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{
				core.TextContent{Text: fmt.Sprintf("Context compacted: %d → %d messages", e.Before, e.After)},
			},
		})
		// Rough estimate until the next EventTurnEnd corrects it
		if e.Before > 0 {
			m.totalIn = (m.totalIn * e.After) / e.Before
			m.updateTokenStatus()
		}
	}

	// Apply actions only after events that can produce them (not stream deltas)
	var actionCmd tea.Cmd
	switch evt.(type) {
	case core.EventAgentEnd, core.EventTurnEnd, core.EventToolEnd, core.EventCompact:
		actionCmd = m.applyActions()
	}

	// Continue polling
	if m.eventCh != nil && m.streaming {
		if actionCmd != nil {
			return m, tea.Batch(pollEvents(m.eventCh), actionCmd)
		}
		return m, pollEvents(m.eventCh)
	}
	return m, actionCmd
}

func (m *Model) updateTokenStatus() {
	m.status.Set(ext.StatusKeyTokens, m.styles.Muted.Render(formatTokens(m.totalIn, m.totalOut, m.totalCacheRead)))
}

// pollEvents reads the next event from the agent channel.
func pollEvents(ch <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			return eventMsg{event: core.EventAgentEnd{}}
		}
		return eventMsg{event: evt}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
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
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{
				core.TextContent{Text: fmt.Sprintf("Background task: %s\n\n%s", m.bgTask, result)},
			},
		})
		m.bgAgent = nil
		m.bgEventCh = nil
		m.bgTask = ""
		m.bgResult.Reset()
		m.status.Set(ext.StatusKeyBg, "")
		return m, nil
	}

	// Continue polling
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
	cmds := app.Commands()
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
