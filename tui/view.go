package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// View implements tea.Model.
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var sections []string

	// Messages viewport
	sections = append(sections, m.viewport.View())

	// Toast notification (transient, above input)
	if m.notification != "" {
		sections = append(sections, m.styles.Muted.Render(" "+m.notification+" "))
	}

	// Input
	sections = append(sections, m.input.View())

	// Status bar
	sections = append(sections, m.status.View())

	// Modal overlay
	if m.modal.Visible() {
		return tea.NewView(m.modal.View())
	}

	v := tea.NewView(m.styles.App.Render(strings.Join(sections, "\n")))
	v.AltScreen = true
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.WindowTitle = m.windowTitle()
	return v
}

// windowTitle returns the terminal window title.
func (m Model) windowTitle() string {
	title := "piglet"
	if m.cfg.Session != nil {
		if name := m.cfg.Session.Meta().Title; name != "" {
			title += " — " + name
		}
	}
	return title
}

// refreshViewport updates the viewport content from messages without changing scroll position.
func (m *Model) refreshViewport() {
	content := "\n" + m.renderMessages()
	contentLines := strings.Count(content, "\n")
	vpHeight := m.viewport.Height()
	if contentLines < vpHeight {
		content = strings.Repeat("\n", vpHeight-contentLines) + content
	}
	m.viewport.SetContent(content)
}

// truncateRunes truncates a string to n runes, appending "..." if truncated.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

// refreshAndFollow updates viewport content and scrolls to bottom if following output.
func (m *Model) refreshAndFollow() {
	m.refreshViewport()
	if m.followOutput {
		m.viewport.GotoBottom()
	}
}

func (m *Model) renderMessages() string {
	var b strings.Builder

	for i, msg := range m.messages {
		if i < len(m.msgCache) && m.msgCache[i] != "" {
			b.WriteString(m.msgCache[i])
		} else {
			rendered := m.msgView.RenderMessage(msg) + "\n\n"
			if len(m.msgCache) <= i {
				m.msgCache = append(m.msgCache, make([]string, i+1-len(m.msgCache))...)
			}
			m.msgCache[i] = rendered
			b.WriteString(rendered)
		}
	}

	// Streaming content
	if m.streaming {
		b.WriteString(m.msgView.RenderStreaming(m.streamText.String(), m.streamThink.String(), &m.streamCache))
	}

	// Active tool indicator
	if m.activeTool != "" {
		b.WriteString(m.styles.ToolName.Render(fmt.Sprintf("running: %s", m.activeTool)))
		b.WriteByte('\n')
	}

	return b.String()
}

func formatTokens(in, out, cacheRead int) string {
	if cacheRead > 0 {
		return fmt.Sprintf("%dk/%dk (cached:%dk)", in/1000, out/1000, cacheRead/1000)
	}
	return fmt.Sprintf("%dk/%dk", in/1000, out/1000)
}

func formatCost(c float64) string {
	if c < 0.01 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", c)
}

// formatImageSize formats a byte count as a human-readable string.
func formatImageSize(bytes int) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
