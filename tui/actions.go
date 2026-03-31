package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/session"
)

func (m Model) handleSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.PushHistory(text)
	m.input.Reset()

	if m.streaming {
		if strings.HasPrefix(text, "/") {
			name, args := parseSlashCommand(text)
			if cmd := m.lookupCommand(name); cmd != nil && cmd.Immediate {
				return m.runCommand(name, args)
			}
		}
		m.enqueueInput(text, strings.HasPrefix(text, "/")) // lowPriority: defer slash commands until agent is idle
		return m, m.notifyAndTick("Queued")
	}

	// Slash command?
	if strings.HasPrefix(text, "/") {
		name, args := parseSlashCommand(text)
		return m.runCommand(name, args)
	}

	// Agent not ready yet?
	if m.cfg.Agent == nil {
		return m, m.notifyAndTick("Extensions loading, try again in a moment")
	}

	// Re-follow output when user sends a message
	m.followOutput = true

	// Send to agent
	userMsg := &core.UserMessage{
		Content:   text,
		Timestamp: time.Now(),
	}
	if m.pendingImage != nil {
		userMsg.Blocks = append(userMsg.Blocks, *m.pendingImage)
		m.pendingImage = nil
		m.input.SetAttachment("")
	}
	m.appendMessage(userMsg)

	m.refreshAndFollow()

	return m, m.startAgentLoop(text)
}

// startAgentLoop runs message hooks and starts the agent, returning the polling batch command.
func (m *Model) startAgentLoop(content string) tea.Cmd {
	if m.cfg.Agent == nil {
		return nil
	}
	if m.cfg.App != nil {
		if injections, err := m.cfg.App.RunMessageHooks(m.ctx, content); err == nil && len(injections) > 0 {
			m.cfg.Agent.SetTurnContext(injections)
		}
	}
	ch := m.cfg.Agent.Start(m.ctx, content)
	m.eventCh = ch
	m.streaming = true
	m.spinnerVerb = "thinking..."
	return tea.Batch(pollEvents(ch), tickCmd(), m.spinner.Tick)
}

// bindApp wires sync callbacks that need return values or direct TUI mutation.
// Fire-and-forget intents (ShowMessage, Quit, etc.) use the action queue.
func (m *Model) bindApp() {
	if m.cfg.App == nil {
		return
	}
	if m.cfg.Agent == nil {
		return
	}
	m.pendingBgStart = nil

	m.cfg.App.Bind(m.cfg.Agent,
		ext.WithRunBackground(func(prompt string) error {
			if m.bgAgent != nil && m.bgAgent.IsRunning() {
				return fmt.Errorf("background task already running")
			}
			tools := m.cfg.App.BackgroundSafeTools()
			bgMax := 5
			if m.cfg.Settings != nil {
				bgMax = config.IntOr(m.cfg.Settings.Agent.BgMaxTurns, 5)
			}
			m.bgAgent = core.NewAgent(core.AgentConfig{
				System:   m.cfg.Agent.System(),
				Provider: m.cfg.Agent.Provider(),
				Tools:    tools,
				MaxTurns: bgMax,
			})
			ch := m.bgAgent.Start(m.ctx, prompt)
			m.bgEventCh = ch
			m.bgTask = prompt
			m.bgResult.Reset()
			task := truncateRunes(prompt, 20)
			m.status.Set(ext.StatusKeyBg, m.styles.Spinner.Render("bg: "+task))
			m.pendingBgStart = &bgStartResult{ch: ch}
			return nil
		}),
		ext.WithCancelBackground(func() {
			m.stopBgAgent()
		}),
		ext.WithIsBackgroundRunning(func() bool {
			return m.bgAgent != nil && m.bgAgent.IsRunning()
		}),
	)
}

// applyActions drains the action queue and applies each action to the model.
// Returns a tea.Cmd if any action requires ongoing work (e.g., background agent polling).
func (m *Model) applyActions() tea.Cmd {
	if m.cfg.App == nil {
		return nil
	}

	var cmds []tea.Cmd
	for _, action := range m.cfg.App.PendingActions() {
		switch action.(type) {
		case ext.ActionShowMessage, ext.ActionNotify,
			ext.ActionShowPicker, ext.ActionAttachImage, ext.ActionDetachImage:
			if cmd := m.applyUIAction(action); cmd != nil {
				cmds = append(cmds, cmd)
			}
		case ext.ActionSetStatus, ext.ActionSwapSession, ext.ActionSetSessionTitle:
			m.applyStateAction(action)
		case ext.ActionRunAsync, ext.ActionExec, ext.ActionQuit, ext.ActionSendMessage:
			if cmd := m.applyAsyncAction(action); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	if m.pendingBgStart != nil {
		ch := m.pendingBgStart.ch
		m.pendingBgStart = nil
		cmds = append(cmds, pollBgEvents(ch))
	}
	return tea.Batch(cmds...)
}

// applyUIAction handles display-only actions: notifications, modals, image attachments.
func (m *Model) applyUIAction(action ext.Action) tea.Cmd {
	switch act := action.(type) {
	case ext.ActionShowMessage:
		return m.notifyAndTick(act.Text)
	case ext.ActionNotify:
		return m.notifyAndTick(act.Message)
	case ext.ActionShowPicker:
		items := make([]ModalItem, len(act.Items))
		for i, item := range act.Items {
			items[i] = ModalItem{ID: item.ID, Label: item.Label, Desc: item.Desc}
		}
		m.modal = NewModalModel(act.Title, items, m.styles)
		m.modal.SetSize(m.width, m.height)
		m.modal.Show()
		m.pickerCallback = act.OnSelect
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
	return nil
}

// applyStateAction handles session and config mutations.
func (m *Model) applyStateAction(action ext.Action) {
	switch act := action.(type) {
	case ext.ActionSetStatus:
		if act.Key == ext.StatusKeyModel {
			m.cfg.Model = findModel(m.cfg.Models, act.Text)
			m.status.Set(ext.StatusKeyModel, m.styles.Muted.Render(act.Text))
		} else {
			m.status.Set(act.Key, m.styles.Muted.Render(act.Text))
		}
	case ext.ActionSwapSession:
		if s, ok := act.Session.(*session.Session); ok {
			if m.cfg.Session != nil {
				_ = m.cfg.Session.Close()
			}
			m.cfg.Session = s
			msgs := s.Messages()
			m.messages = msgs
			m.msgCache = nil
			if m.cfg.Agent != nil {
				m.cfg.Agent.SetMessages(msgs)
			}
		}
	case ext.ActionSetSessionTitle:
		if m.cfg.Session != nil && act.Title != "" {
			_ = m.cfg.Session.SetTitle(act.Title)
		}
	}
}

// applyAsyncAction handles actions that produce tea.Cmds or affect lifecycle.
func (m *Model) applyAsyncAction(action ext.Action) tea.Cmd {
	switch act := action.(type) {
	case ext.ActionRunAsync:
		fn := act.Fn
		return func() tea.Msg {
			result := fn()
			if result != nil {
				return asyncActionMsg{action: result}
			}
			return nil
		}
	case ext.ActionExec:
		if c, ok := act.Cmd.(*exec.Cmd); ok {
			return tea.ExecProcess(c, func(err error) tea.Msg {
				return execDoneMsg{err: err}
			})
		}
	case ext.ActionQuit:
		m.stopBgAgent()
		m.quitting = true
	case ext.ActionSendMessage:
		if m.streaming {
			m.enqueueInput(act.Content, false) // lowPriority: false — this is a direct message, send ASAP
			return m.notifyAndTick("Queued")
		}
		content := act.Content
		m.followOutput = true
		userMsg := &core.UserMessage{
			Content:   content,
			Timestamp: time.Now(),
		}
		m.appendMessage(userMsg)
		m.refreshAndFollow()
		return m.startAgentLoop(content)
	}
	return nil
}

// stopBgAgent cancels the background agent if one is running.
func (m *Model) stopBgAgent() {
	if m.bgAgent != nil && m.bgAgent.IsRunning() {
		m.bgAgent.Stop()
	}
	m.bgAgent = nil
	m.bgEventCh = nil
	m.bgTask = ""
	m.bgResult.Reset()
	m.status.Set(ext.StatusKeyBg, "")
}

// lookupCommand finds a registered command by name. Returns nil if not found.
func (m Model) lookupCommand(name string) *ext.Command {
	if m.cfg.App == nil {
		return nil
	}
	cmds := m.cfg.App.Commands()
	cmd, ok := cmds[name]
	if !ok {
		return nil
	}
	return cmd
}

// runCommand dispatches a slash command to the registered handler.
func (m Model) runCommand(name, args string) (tea.Model, tea.Cmd) {
	if m.cfg.App == nil {
		return m, nil
	}

	// Alias
	if name == "exit" {
		name = "quit"
	}

	cmds := m.cfg.App.Commands()
	cmd, ok := cmds[name]
	if !ok {
		m.messages = append(m.messages, systemNote("Unknown command: /"+name))
		return m, nil
	}

	// Bind callbacks, run handler, apply results
	m.bindApp()

	// Special handling for /clear: clear messages before handler runs
	if name == "clear" {
		m.messages = nil
		m.msgCache = nil
		if m.cfg.Agent != nil {
			m.cfg.Agent.SetMessages(nil)
		}
	}

	if err := cmd.Handler(args, m.cfg.App); err != nil {
		m.messages = append(m.messages, systemNote("Command error: "+err.Error()))
		return m, nil
	}

	bgCmd := m.applyActions()

	if m.quitting {
		return m, tea.Quit
	}

	return m, bgCmd
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

	m.bindApp()
	action, _ := sc.Handler(m.cfg.App)
	if action != nil {
		m.cfg.App.EnqueueAction(action)
	}
	cmd := m.applyActions()

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

// parseSlashCommand splits "/name arg1 arg2" into ("name", "arg1 arg2").
func parseSlashCommand(text string) (name, args string) {
	text = strings.TrimPrefix(text, "/")
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", ""
	}
	name = parts[0]
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}
	return
}
