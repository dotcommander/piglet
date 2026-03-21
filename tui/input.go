package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// InputModel manages the composer textarea with slash command autocomplete.
type InputModel struct {
	textarea textarea.Model
	styles   Styles
	width    int

	// Slash autocomplete
	suggestions []string
	selected    int
	showing     bool
	commands    []string // all registered command names
}

// NewInputModel creates a new composer input.
func NewInputModel(styles Styles, commands []string) InputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (/ for commands)"
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	s := ta.Styles()
	s.Focused.CursorLine = styles.App
	ta.SetStyles(s)

	ta.Focus()

	return InputModel{
		textarea: ta,
		styles:   styles,
		commands: commands,
	}
}

// SetWidth updates the input width.
func (m *InputModel) SetWidth(w int) {
	m.width = w
	inner := w - 4 // border + padding
	if inner < 10 {
		inner = 10
	}
	m.textarea.SetWidth(inner)
}

// Focus focuses the textarea.
func (m *InputModel) Focus() { m.textarea.Focus() }

// Blur unfocuses the textarea.
func (m *InputModel) Blur() { m.textarea.Blur() }

// Value returns the current text.
func (m *InputModel) Value() string { return m.textarea.Value() }

// SetValue sets the textarea content.
func (m *InputModel) SetValue(s string) { m.textarea.SetValue(s) }

// Reset clears the textarea.
func (m *InputModel) Reset() { m.textarea.Reset() }

// Update handles input events.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Filter out terminal response sequences that leak through as key events.
		// These contain raw escape codes (OSC, CSI) that shouldn't reach the textarea.
		if msg.Text != "" && strings.ContainsAny(msg.Text, "\x1b\x9c") {
			return m, nil
		}
		switch {
		case msg.Code == tea.KeyTab:
			if m.showing && len(m.suggestions) > 0 {
				m.textarea.SetValue("/" + m.suggestions[m.selected] + " ")
				m.showing = false
				return m, nil
			}
		case msg.Code == tea.KeyUp:
			if m.showing && m.selected > 0 {
				m.selected--
				return m, nil
			}
		case msg.Code == tea.KeyDown:
			if m.showing && m.selected < len(m.suggestions)-1 {
				m.selected++
				return m, nil
			}
		case msg.Code == tea.KeyEscape:
			if m.showing {
				m.showing = false
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)

	// Update autocomplete
	val := m.textarea.Value()
	if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") {
		prefix := strings.TrimPrefix(val, "/")
		m.suggestions = m.matchCommands(prefix)
		m.showing = len(m.suggestions) > 0
		if m.selected >= len(m.suggestions) {
			m.selected = 0
		}
	} else {
		m.showing = false
	}

	return m, cmd
}

// View renders the input.
func (m InputModel) View() string {
	var b strings.Builder

	if m.showing && len(m.suggestions) > 0 {
		for i, s := range m.suggestions {
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			b.WriteString(m.styles.Muted.Render(prefix+"/"+s) + "\n")
		}
	}

	b.WriteString(m.styles.InputBorder.Width(m.width - 4).Render(m.textarea.View()))
	return b.String()
}

func (m InputModel) matchCommands(prefix string) []string {
	if prefix == "" {
		return m.commands
	}
	var matches []string
	for _, cmd := range m.commands {
		if strings.HasPrefix(cmd, prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}
