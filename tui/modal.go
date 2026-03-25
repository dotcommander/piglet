package tui

import (
	"slices"
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
				r := []rune(m.filter)
				m.filter = string(r[:len(r)-1])
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
		b.WriteString(m.styles.Muted.Render("filter: " + m.filter + "_"))
	} else {
		b.WriteString(m.styles.Muted.Render("filter: _"))
	}
	b.WriteByte('\n')
	b.WriteByte('\n')

	// Items
	visible := m.filtered
	scrollOff := 0
	if len(visible) > maxH {
		scrollOff = scrollStart(m.cursor, len(visible), maxH)
		visible = visible[scrollOff : scrollOff+maxH]
	}

	for i, item := range visible {
		prefix := "  "
		idx := scrollOff + i

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
		if m.cursor >= len(m.filtered) {
			m.cursor = max(0, len(m.filtered)-1)
		}
		return
	}

	lf := []rune(strings.ToLower(m.filter))
	type scored struct {
		item  ModalItem
		score int
	}
	var matches []scored
	for _, item := range m.items {
		best := -1
		for _, text := range []string{item.Label, item.Desc, item.ID} {
			if s, ok := fuzzyScore([]rune(strings.ToLower(text)), lf); ok {
				if best < 0 || s < best {
					best = s
				}
			}
		}
		if best >= 0 {
			matches = append(matches, scored{item: item, score: best})
		}
	}
	slices.SortFunc(matches, func(a, b scored) int { return a.score - b.score })
	m.filtered = make([]ModalItem, len(matches))
	for i, s := range matches {
		m.filtered[i] = s.item
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// fuzzyScore checks if needle characters appear in order within haystack.
// Returns the sum of gaps between consecutive matched positions (lower = tighter match)
// and true if all characters matched, or -1 and false otherwise.
func fuzzyScore(haystack, needle []rune) (int, bool) {
	if len(needle) == 0 {
		return 0, true
	}
	hi := 0
	prevPos := -1
	score := 0
	for _, nr := range needle {
		found := false
		for hi < len(haystack) {
			r := haystack[hi]
			pos := hi
			hi++
			if r == nr {
				if prevPos >= 0 {
					score += pos - prevPos - 1
				}
				prevPos = pos
				found = true
				break
			}
		}
		if !found {
			return -1, false
		}
	}
	return score, true
}

// scrollStart computes the first visible index for a centered scroll window.
func scrollStart(cursor, items, height int) int {
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	if start+height > items {
		start = items - height
	}
	return start
}

// themeColors returns a Theme with default border color.
func (s Styles) styles() Theme {
	return Theme{Border: lipgloss.Color("#45475A")}
}
