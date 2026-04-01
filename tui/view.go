package tui

import (
	"fmt"
	"strconv"
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
		style := m.notificationStyle()
		sections = append(sections, style.Render(" "+m.notification+" "))
	}

	// Extension widgets: above-input placement
	if w := m.renderWidgets("above-input"); w != "" {
		sections = append(sections, w)
	}

	// Input
	sections = append(sections, m.input.View())

	// Status bar
	sections = append(sections, m.status.View())

	// Extension widgets: below-status placement
	if w := m.renderWidgets("below-status"); w != "" {
		sections = append(sections, w)
	}

	// Modal overlay
	if m.modal.Visible() {
		return tea.NewView(m.modal.View())
	}

	// Extension overlays (stacked, topmost wins)
	if m.overlays.Visible() {
		return tea.NewView(m.overlays.View())
	}

	v := tea.NewView(m.styles.App.Render(strings.Join(sections, "\n")))
	v.AltScreen = true
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.WindowTitle = m.windowTitle()
	return v
}

// maxWidgetLines caps each individual widget to prevent extensions from eating the viewport.
const maxWidgetLines = 5

// maxTotalWidgetLines caps the total widget budget across all widgets in a placement.
const maxTotalWidgetLines = 10

// renderWidgets renders all widgets for a given placement, capped to budget.
func (m Model) renderWidgets(placement string) string {
	var lines []string
	total := 0
	for _, w := range m.widgets {
		if w.Placement != placement {
			continue
		}
		wLines := strings.Split(w.Content, "\n")
		if len(wLines) > maxWidgetLines {
			wLines = wLines[:maxWidgetLines]
		}
		for _, l := range wLines {
			if total >= maxTotalWidgetLines {
				break
			}
			lines = append(lines, l)
			total++
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return m.styles.Muted.Render(strings.Join(lines, "\n"))
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

func fmtK(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	case n >= 1000:
		return strconv.Itoa(n/1000) + "k"
	default:
		return strconv.Itoa(n)
	}
}

func formatTokens(in, out, cacheRead int) string {
	if cacheRead > 0 {
		return fmt.Sprintf("%s/%s (cached:%s)", fmtK(in), fmtK(out), fmtK(cacheRead))
	}
	return fmt.Sprintf("%s/%s", fmtK(in), fmtK(out))
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
