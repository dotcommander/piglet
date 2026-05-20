package tui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()

	case tea.KeyPressMsg:
		if result, cmd, handled := m.handleKeyPress(msg); handled {
			return result, cmd
		}

	case tea.ResumeMsg:
		return m, m.input.textarea.Focus()

	case tea.FocusMsg:
		m.focused = true
		return m, nil

	case tea.BlurMsg:
		m.focused = false
		return m, nil

	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)

	case tea.MouseWheelMsg:
		m.viewport, _ = m.viewport.Update(msg)
		m.followOutput = m.viewport.AtBottom()
		return m, nil

	case eventMsg:
		return m.handleEvent(msg.event, false)

	case eventsBatchMsg:
		return m.handleEventsBatch(msg)

	case tickMsg:
		if m.streaming {
			m.refreshAndFollow()
			cmds = append(cmds, tickCmd())
		}

	case ModalSelectMsg:
		return m.handleModalSelect(msg)

	case ModalCloseMsg:
		m.pickerCallback = nil
		m.modalAction = nil
		return m, nil

	case ModalAskCancelMsg:
		return m.handleModalAskCancel()

	case bgEventMsg:
		return m.handleBgEvent(msg.event)

	case notifyTickMsg:
		return m.handleNotifyTick()

	case asyncActionMsg:
		return m.handleAsyncAction(msg)

	case execDoneMsg:
		if msg.err != nil {
			cmds = append(cmds, m.notifyAndTick("editor: "+msg.err.Error()))
		}
		return m, tea.Batch(cmds...)

	case AgentReadyMsg:
		return m.handleAgentReady(msg)

	case bashTailMsg:
		// Stale lines from a finished call are ignored; always re-arm the drain.
		if m.applyBashTail(msg) {
			m.refreshAndFollow()
		}
		return m, drainBashTail(m.bashTailCh)
	}

	// Update input
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	if inputCmd != nil {
		cmds = append(cmds, inputCmd)
	}

	return m, tea.Batch(cmds...)
}
