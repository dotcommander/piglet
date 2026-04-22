package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dotcommander/piglet/core"
)

// stopStreaming resets all streaming-related state and transitions to idle.
func (m *Model) stopStreaming() {
	m.streaming = false
	m.activeTool = ""
	m.spinnerVerb = ""
	m.status.SetSpinnerView("")
	m.streamCache = streamCache{}
	m.refreshAndFollow()
	if m.cfg.App != nil {
		m.cfg.App.SignalIdle()
	}
}

func (m *Model) layout() {
	statusH := 1
	inputH := 5 // border + 3 lines + border
	vpH := m.height - statusH - inputH - 2
	if vpH < 3 {
		vpH = 3
	}

	wasAtBottom := m.followOutput

	// Resize viewport geometry
	m.viewport.SetWidth(m.width - 2)
	m.viewport.SetHeight(vpH)

	// Update renderers and invalidate cache BEFORE refresh so the first
	// post-resize frame renders at the new width, not the stale cached width.
	m.input.SetWidth(m.width)
	m.status.SetWidth(m.width - 2) // subtract App padding
	m.msgView.SetWidth(m.width - 2)
	m.msgCache = nil
	m.modal.SetSize(m.width, m.height)
	m.overlays.SetSize(m.width, m.height)

	// Now render at the new width
	m.refreshViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

func (m *Model) showNotification(text string) {
	m.notification = text
	m.notificationLevel = ""
	m.notificationTimer = notifyDuration
}

// notificationStyle returns the lipgloss style for the current notification level.
func (m Model) notificationStyle() lipgloss.Style {
	switch m.notificationLevel {
	case "warn":
		return m.styles.Warning
	case "error":
		return m.styles.Error
	default:
		return m.styles.Muted
	}
}

// appendDisplayMessage adds a message to the TUI display list.
// Persistence is handled by shell.ProcessEvent.
func (m *Model) appendDisplayMessage(msg core.Message) {
	m.messages = append(m.messages, msg)
}

// notifyAndTick shows a notification and returns the tick command to dismiss it.
func (m *Model) notifyAndTick(text string) tea.Cmd {
	m.showNotification(text)
	return notifyTick()
}

func notifyTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return notifyTickMsg{}
	})
}
