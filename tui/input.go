package tui

import (
	"bufio"
	"log/slog"
	"os"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/config"
)

const maxHistory = 500

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

	attachment string // shown above the input border when non-empty

	// Input history (up/down arrow cycles like bash, persisted to disk)
	history     []string
	histIdx     int    // current position; len(history) = "new input"
	draft       string // saves in-progress text when entering history
	historyPath string // file path for persistent history
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

	km := ta.KeyMap
	km.InsertNewline.SetKeys("alt+enter")
	ta.KeyMap = km

	return InputModel{
		textarea: ta,
		styles:   styles,
		commands: commands,
	}
}

// LoadHistory reads persistent history from disk and sets the path for
// future saves. Missing file is a no-op.
func (m *InputModel) LoadHistory(path string) {
	m.historyPath = path
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("load input history", "error", err)
		}
		return
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		slog.Warn("read input history", "error", err)
		return
	}
	if len(lines) > maxHistory {
		lines = lines[len(lines)-maxHistory:]
	}
	m.history = lines
	m.histIdx = len(lines)
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

// PushHistory adds an entry to the input history, resets the cursor,
// and persists the history to disk.
func (m *InputModel) PushHistory(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	// Deduplicate consecutive entries
	if len(m.history) > 0 && m.history[len(m.history)-1] == s {
		m.histIdx = len(m.history)
		return
	}
	if len(m.history) >= maxHistory {
		m.history = m.history[1:]
	}
	m.history = append(m.history, s)
	m.histIdx = len(m.history)
	m.saveHistory()
}

func (m *InputModel) saveHistory() {
	if m.historyPath == "" {
		return
	}
	data := strings.Join(m.history, "\n") + "\n"
	if err := config.AtomicWrite(m.historyPath, []byte(data), 0o600); err != nil {
		slog.Warn("save input history", "error", err)
	}
}

// SetAttachment sets the attachment indicator shown above the input.
func (m *InputModel) SetAttachment(s string) { m.attachment = s }

// SetCommands updates the registered command names for autocomplete.
func (m *InputModel) SetCommands(cmds []string) { m.commands = cmds }

// Update handles input events.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
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
			if !m.showing && len(m.history) > 0 && m.textarea.LineCount() <= 1 {
				if m.histIdx == len(m.history) {
					m.draft = m.textarea.Value()
				}
				if m.histIdx > 0 {
					m.histIdx--
					m.textarea.SetValue(m.history[m.histIdx])
				}
				return m, nil
			}
		case msg.Code == tea.KeyDown:
			if m.showing && m.selected < len(m.suggestions)-1 {
				m.selected++
				return m, nil
			}
			if !m.showing && m.histIdx < len(m.history) && m.textarea.LineCount() <= 1 {
				m.histIdx++
				if m.histIdx == len(m.history) {
					m.textarea.SetValue(m.draft)
				} else {
					m.textarea.SetValue(m.history[m.histIdx])
				}
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

	if m.attachment != "" {
		b.WriteString(m.styles.Spinner.Render("  [image attached]") + "\n")
	}

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
