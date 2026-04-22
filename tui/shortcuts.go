package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

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
