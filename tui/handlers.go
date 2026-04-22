package tui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/ext"
)

// handleSpinnerTick advances the spinner animation during streaming.
func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.streaming {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.status.SetSpinnerView(m.spinner.View() + " " + m.spinnerVerb)
		return m, cmd
	}
	return m, nil
}

// handleEventsBatch processes a batch of agent events, emitting a single
// pollEvents at the end rather than one per event.
func (m Model) handleEventsBatch(msg eventsBatchMsg) (tea.Model, tea.Cmd) {
	// nil events means the channel was closed — stop streaming.
	if msg.events == nil {
		m.stopStreaming()
		return m, nil
	}

	var cmds []tea.Cmd
	var model tea.Model = m
	for _, evt := range msg.events {
		var cmd tea.Cmd
		model, cmd = m.handleEvent(evt, true)
		m = model.(Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Single pollEvents for the entire batch
	if m.eventCh != nil && m.streaming {
		cmds = append(cmds, pollEvents(m.eventCh))
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// handleModalAskCancel fires the askUser callback with Cancelled=true and drains actions.
func (m Model) handleModalAskCancel() (tea.Model, tea.Cmd) {
	if m.askUserCallback != nil {
		cb := m.askUserCallback
		m.askUserCallback = nil
		cb(ext.AskUserResult{Cancelled: true})
		if m.shell != nil {
			m.shell.DrainActions()
		}
		if bgCmd := m.applyShellNotifications(); bgCmd != nil {
			return m, bgCmd
		}
	}
	return m, nil
}

// handleModalSelect fires the appropriate callback (askUser or picker) and drains
// shell actions. Exactly one of askUserCallback or pickerCallback is set when the
// modal is visible — modal is single-use per mount.
func (m Model) handleModalSelect(msg ModalSelectMsg) (tea.Model, tea.Cmd) {
	if m.askUserCallback != nil {
		cb := m.askUserCallback
		m.askUserCallback = nil
		cb(ext.AskUserResult{Selected: msg.Item.ID})
	} else if m.pickerCallback != nil {
		cb := m.pickerCallback
		m.pickerCallback = nil
		cb(ext.PickerItem{
			ID:    msg.Item.ID,
			Label: msg.Item.Label,
			Desc:  msg.Item.Desc,
		})
	} else {
		return m, nil
	}
	if m.shell != nil {
		m.shell.DrainActions()
	}
	if bgCmd := m.applyShellNotifications(); bgCmd != nil {
		return m, bgCmd
	}
	return m, nil
}

// handleNotifyTick decrements the notification countdown and clears when done.
func (m Model) handleNotifyTick() (tea.Model, tea.Cmd) {
	if m.notificationTimer > 0 {
		m.notificationTimer--
		if m.notificationTimer > 0 {
			return m, notifyTick()
		}
		m.notification = ""
	}
	return m, nil
}

// handleAsyncAction re-enqueues the result from an ActionRunAsync and applies it.
func (m Model) handleAsyncAction(msg asyncActionMsg) (tea.Model, tea.Cmd) {
	if m.shell != nil && msg.action != nil {
		m.shell.EnqueueResult(msg.action)
		m.shell.DrainActions()
		if bgCmd := m.applyShellNotifications(); bgCmd != nil {
			return m, bgCmd
		}
	}
	return m, nil
}

// handleAgentReady wires the fully-initialized agent into the model after
// background setup completes.
func (m Model) handleAgentReady(msg AgentReadyMsg) (tea.Model, tea.Cmd) {
	m.cfg.Agent = msg.Agent
	m.cfg.SetupFn = nil
	if m.shell != nil {
		m.shell.SetAgent(msg.Agent)
	}
	m.status.Set(ext.StatusKeyApp, m.styles.Muted.Render("piglet"))
	if m.cfg.App != nil {
		m.status.SetRegistry(m.cfg.App.StatusSections())
		var sugs []CommandSuggestion
		for _, d := range m.shell.CommandInfos() {
			sugs = append(sugs, CommandSuggestion{Name: d.Name, Description: d.Description})
		}
		m.input.SetCommands(sugs)
	}
	return m, m.notifyAndTick("Extensions loaded")
}

// handleKeyPress processes keyboard input. Returns handled=true if the key
// was consumed (modal, global shortcut, submit). When handled=false the
// caller should let the input textarea handle the key.
func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.modal.Visible() {
		var cmd tea.Cmd
		m.modal, cmd = m.modal.Update(msg)
		return m, cmd, true
	}

	if m.overlays.Visible() {
		switch {
		case msg.Code == tea.KeyEscape:
			m.overlays.DismissTop()
			return m, nil, true
		case msg.Code == tea.KeyUp:
			m.overlays.ScrollUp()
			return m, nil, true
		case msg.Code == tea.KeyDown:
			m.overlays.ScrollDown()
			return m, nil, true
		}
		return m, nil, true // consume all other keys while overlay visible
	}

	switch {
	case msg.Code == tea.KeyEscape:
		if m.streaming && m.shell != nil {
			m.shell.Abort()
			return m, nil, true
		}
		return m, nil, false

	case msg.Code == 'c' && msg.Mod.Contains(tea.ModCtrl):
		if m.streaming && m.shell != nil {
			m.shell.Abort()
			return m, nil, true
		}
		if m.shell != nil {
			m.shell.StopBackground()
		}
		m.quitting = true
		return m, tea.Quit, true

	case msg.Code == tea.KeyPgUp, msg.Code == tea.KeyPgDown:
		m.viewport, _ = m.viewport.Update(msg)
		m.followOutput = m.viewport.AtBottom()
		return m, nil, true

	case msg.Code == tea.KeyEnter && !msg.Mod.Contains(tea.ModAlt):
		result, cmd := m.handleSubmit()
		return result, cmd, true

	case msg.Code == 'm' && msg.Mod.Contains(tea.ModCtrl):
		// Delegate to /mouse so the toggle persists to config — single source of truth.
		if m.shell != nil {
			m.shell.Submit("/mouse")
		}
		return m, m.applyShellNotifications(), true

	case msg.Code == 'z' && msg.Mod.Contains(tea.ModCtrl):
		return m, tea.Suspend, true

	default:
		if result, cmd, handled := m.runShortcut(msg); handled {
			return result, cmd, true
		}
	}

	return m, nil, false
}
