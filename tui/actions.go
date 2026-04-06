package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/session"
	"github.com/dotcommander/piglet/shell"
)

func (m Model) handleSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.PushHistory(text)
	m.input.Reset()

	if m.shell == nil {
		return m, m.notifyAndTick("Shell not initialized")
	}

	// If it's a regular message (not a command), re-follow output and track image
	if !strings.HasPrefix(text, "/") && !m.streaming {
		m.followOutput = true
	}

	// Handle pending image attachment
	var img *core.ImageContent
	if m.pendingImage != nil && !strings.HasPrefix(text, "/") {
		img = m.pendingImage
		m.pendingImage = nil
		m.input.SetAttachment("")
	}

	var resp shell.Response
	if img != nil {
		resp = m.shell.SubmitWithImage(text, img)
	} else {
		resp = m.shell.Submit(text)
	}

	// Apply any notifications from the submit
	notifyCmd := m.applyShellNotifications()

	switch resp.Kind {
	case shell.ResponseAgentStarted:
		// Add user message to display
		m.appendDisplayMessage(&core.UserMessage{Content: text, Timestamp: time.Now()})
		m.refreshAndFollow()
		return m, tea.Batch(m.startStreaming(resp), notifyCmd)

	case shell.ResponseQueued:
		return m, tea.Batch(m.notifyAndTick("Queued"), notifyCmd)

	case shell.ResponseCommand:
		return m, notifyCmd

	case shell.ResponseHandled:
		return m, tea.Batch(m.notifyAndTick("Input handled"), notifyCmd)

	case shell.ResponseNotReady:
		return m, tea.Batch(m.notifyAndTick("Extensions loading, try again in a moment"), notifyCmd)

	case shell.ResponseError:
		return m, tea.Batch(m.notifyAndTick(resp.Error.Error()), notifyCmd)
	}

	return m, notifyCmd
}

// applyShellNotifications drains shell notifications and translates them
// into TUI state mutations and tea.Cmds.
func (m *Model) applyShellNotifications() tea.Cmd {
	if m.shell == nil {
		return nil
	}

	var cmds []tea.Cmd
	for _, n := range m.shell.Notifications() {
		if cmd := m.applyNotification(n); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if m.quitting {
		cmds = append(cmds, tea.Quit)
	}

	// Check if shell started a background agent
	for name, ch := range m.shell.BgEventChannels() {
		if ch != m.bgEventCh {
			m.bgEventCh = ch
			m.bgTaskName = name
			cmds = append(cmds, pollBgEvents(ch))
			break // TUI tracks one bg task at a time for now
		}
	}

	return tea.Batch(cmds...)
}

// applyNotification translates a single shell.Notification into TUI state changes.
func (m *Model) applyNotification(n shell.Notification) tea.Cmd {
	switch n.Kind {
	case shell.NotifyMessage:
		return m.notifyAndTick(n.Text)

	case shell.NotifyWarn:
		m.notification = n.Text
		m.notificationLevel = "warn"
		m.notificationTimer = notifyDuration
		return notifyTick()

	case shell.NotifyError:
		m.notification = n.Text
		m.notificationLevel = "error"
		m.notificationTimer = notifyDuration
		return notifyTick()

	case shell.NotifyStatus:
		if n.Key == ext.StatusKeyModel {
			m.cfg.Model = findModel(m.cfg.Models, n.Text)
			m.status.Set(ext.StatusKeyModel, m.styles.Muted.Render(n.Text))
		} else if n.Text == "" {
			m.status.Set(n.Key, "")
		} else {
			m.status.Set(n.Key, m.styles.Muted.Render(n.Text))
		}

	case shell.NotifySessionSwap:
		if act, ok := n.Action.(ext.ActionSwapSession); ok {
			if s, ok := act.Session.(*session.Session); ok {
				m.cfg.Session = s
				m.messages = s.Messages()
				m.msgCache = nil
			}
		}

	case shell.NotifyQuit:
		m.quitting = true

	case shell.NotifyPicker:
		if act, ok := n.Action.(ext.ActionShowPicker); ok {
			items := make([]ModalItem, len(act.Items))
			for i, item := range act.Items {
				items[i] = ModalItem{ID: item.ID, Label: item.Label, Desc: item.Desc}
			}
			m.modal = NewModalModel(act.Title, items, m.styles)
			m.modal.SetSize(m.width, m.height)
			m.modal.Show()
			m.pickerCallback = act.OnSelect
		}

	case shell.NotifyImage:
		switch act := n.Action.(type) {
		case ext.ActionAttachImage:
			if m.pendingImage != nil {
				m.pendingImage = nil
				m.input.SetAttachment("")
				return m.notifyAndTick("Image attachment removed")
			} else if img, ok := act.Image.(*core.ImageContent); ok {
				m.pendingImage = img
				m.input.SetAttachment("image")
				size := len(img.Data) * 3 / 4
				return m.notifyAndTick(fmt.Sprintf("Image attached (%s) — send with your next message", formatImageSize(size)))
			}
		case ext.ActionDetachImage:
			m.pendingImage = nil
			m.input.SetAttachment("")
			return m.notifyAndTick("Image attachment removed")
		}

	case shell.NotifyWidget:
		if act, ok := n.Action.(ext.ActionSetWidget); ok {
			if act.Content == "" {
				delete(m.widgets, act.Key)
			} else {
				m.widgets[act.Key] = widgetState{
					Placement: act.Placement,
					Content:   act.Content,
				}
			}
		}

	case shell.NotifyOverlay:
		switch act := n.Action.(type) {
		case ext.ActionShowOverlay:
			m.overlays.Show(act.Key, act.Title, act.Content, act.Anchor, act.Width)
		case ext.ActionCloseOverlay:
			m.overlays.Close(act.Key)
		}

	case shell.NotifyExec:
		if act, ok := n.Action.(ext.ActionExec); ok {
			if c, ok := act.Cmd.(*exec.Cmd); ok {
				return tea.ExecProcess(c, func(err error) tea.Msg {
					return execDoneMsg{err: err}
				})
			}
		}

	case shell.NotifySendMessage:
		// Re-submit through shell. If we're not streaming, submit directly.
		if !m.streaming && m.shell != nil {
			m.followOutput = true
			content := n.Text
			m.appendDisplayMessage(&core.UserMessage{Content: content, Timestamp: time.Now()})
			m.refreshAndFollow()
			resp := m.shell.Submit(content)
			if resp.Kind == shell.ResponseAgentStarted {
				return m.startStreaming(resp)
			}
		}

	case shell.NotifyQueuedSubmit:
		m.followOutput = true
		m.appendDisplayMessage(&core.UserMessage{Content: n.Text, Timestamp: time.Now()})
		m.refreshAndFollow()

	case shell.NotifyClearDisplay:
		m.messages = nil
		m.msgCache = nil

	case shell.NotifySessionTitle:
		// No TUI-specific action needed; shell already persisted it
	}

	return nil
}

// startStreaming transitions the model into streaming state for the given response
// and returns the batch of commands needed to drive the streaming loop.
func (m *Model) startStreaming(resp shell.Response) tea.Cmd {
	m.eventCh = resp.Events
	m.streaming = true
	m.spinnerVerb = "thinking..."
	return tea.Batch(pollEvents(resp.Events), tickCmd(), m.spinner.Tick)
}

// runShortcut checks if the key matches a registered shortcut and runs it.
func (m *Model) runShortcut(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.cfg.App == nil {
		return m, nil, false
	}

	key := keyString(msg)
	if key == "" {
		return m, nil, false
	}

	shortcuts := m.cfg.App.Shortcuts()
	sc, ok := shortcuts[key]
	if !ok {
		return m, nil, false
	}

	action, _ := sc.Handler(m.cfg.App)
	if action != nil {
		m.cfg.App.EnqueueAction(action)
	}
	// Drain actions through shell
	if m.shell != nil {
		m.shell.DrainActions()
	}
	cmd := m.applyShellNotifications()

	return m, cmd, true
}

// keyString converts a KeyPressMsg to the shortcut key format (e.g. "ctrl+p").
func keyString(msg tea.KeyPressMsg) string {
	if !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt) {
		return ""
	}
	var parts []string
	if msg.Mod.Contains(tea.ModCtrl) {
		parts = append(parts, "ctrl")
	}
	if msg.Mod.Contains(tea.ModAlt) {
		parts = append(parts, "alt")
	}
	if msg.Mod.Contains(tea.ModShift) {
		parts = append(parts, "shift")
	}
	if msg.Code >= 'a' && msg.Code <= 'z' {
		parts = append(parts, string(msg.Code))
	} else {
		return ""
	}
	return strings.Join(parts, "+")
}
