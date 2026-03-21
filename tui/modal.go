package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ModalItem is an item in a modal list.
type ModalItem struct {
	ID    string
	Label string
	Desc  string
}

// ModalModel is a generic list selector modal.
type ModalModel struct {
	title    string
	items    []ModalItem
	filtered []ModalItem
	filter   string
	cursor   int
	width    int
	height   int
	styles   Styles
	visible  bool
}

// NewModalModel creates a modal.
func NewModalModel(title string, items []ModalItem, styles Styles) ModalModel {
	return ModalModel{
		title:    title,
		items:    items,
		filtered: items,
		styles:   styles,
	}
}

// ModalSelectMsg is sent when an item is selected.
type ModalSelectMsg struct {
	Item ModalItem
}

// ModalCloseMsg is sent when the modal is dismissed.
type ModalCloseMsg struct{}

// Show makes the modal visible.
func (m *ModalModel) Show() { m.visible = true; m.cursor = 0; m.filter = ""; m.filtered = m.items }

// Hide hides the modal.
func (m *ModalModel) Hide() { m.visible = false }

// Visible returns whether the modal is shown.
func (m ModalModel) Visible() bool { return m.visible }

// SetItems updates the items list.
func (m *ModalModel) SetItems(items []ModalItem) {
	m.items = items
	m.applyFilter()
}

// SetSize updates the modal dimensions.
func (m *ModalModel) SetSize(w, h int) { m.width = w; m.height = h }

// Update handles modal events.
func (m ModalModel) Update(msg tea.Msg) (ModalModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case msg.Code == tea.KeyEscape:
			m.visible = false
			return m, func() tea.Msg { return ModalCloseMsg{} }
		case msg.Code == tea.KeyEnter:
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				item := m.filtered[m.cursor]
				m.visible = false
				return m, func() tea.Msg { return ModalSelectMsg{Item: item} }
			}
		case msg.Code == tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case msg.Code == tea.KeyDown:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case msg.Code == tea.KeyBackspace:
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.applyFilter()
			}
		default:
			if msg.Text != "" {
				m.filter += msg.Text
				m.applyFilter()
			}
		}
	}

	return m, nil
}

// View renders the modal.
func (m ModalModel) View() string {
	if !m.visible {
		return ""
	}

	w := m.width - 8
	if w < 30 {
		w = 30
	}
	maxH := m.height - 6
	if maxH < 5 {
		maxH = 5
	}

	var b strings.Builder

	// Title
	b.WriteString(m.styles.Header.Render(m.title))
	b.WriteByte('\n')

	// Filter
	if m.filter != "" {
		b.WriteString(m.styles.Muted.Render("filter: " + m.filter))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	// Items
	visible := m.filtered
	if len(visible) > maxH {
		start := m.cursor - maxH/2
		if start < 0 {
			start = 0
		}
		if start+maxH > len(visible) {
			start = len(visible) - maxH
		}
		visible = visible[start : start+maxH]
	}

	for i, item := range visible {
		prefix := "  "
		idx := i
		// Adjust index if we scrolled
		if len(m.filtered) > maxH {
			start := m.cursor - maxH/2
			if start < 0 {
				start = 0
			}
			if start+maxH > len(m.filtered) {
				start = len(m.filtered) - maxH
			}
			idx = start + i
		}

		if idx == m.cursor {
			prefix = "> "
		}

		label := item.Label
		if item.Desc != "" {
			label += m.styles.Muted.Render(" — " + item.Desc)
		}
		b.WriteString(prefix + label + "\n")
	}

	// Help
	b.WriteByte('\n')
	b.WriteString(m.styles.Muted.Render("move: up/down | select: enter | close: esc | filter: type"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.styles().Border).
		Padding(1, 2).
		Width(w)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box.Render(b.String()))
}

func (m *ModalModel) applyFilter() {
	if m.filter == "" {
		m.filtered = m.items
	} else {
		lf := strings.ToLower(m.filter)
		m.filtered = nil
		for _, item := range m.items {
			if strings.Contains(strings.ToLower(item.Label), lf) ||
				strings.Contains(strings.ToLower(item.Desc), lf) ||
				strings.Contains(strings.ToLower(item.ID), lf) {
				m.filtered = append(m.filtered, item)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// themeColors returns a Theme with default border color.
func (s Styles) styles() Theme {
	return Theme{Border: lipgloss.Color("#45475A")}
}
