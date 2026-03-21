package tui

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/dotcommander/piglet/ext"
)

// StatusBar renders the footer status bar.
// Sections are registered through ext.RegisterStatusSection.
// Values are set via Set(key, value).
type StatusBar struct {
	sections map[string]string   // key → rendered value
	registry []ext.StatusSection // registered section metadata
	thinking bool
	spinning bool
	width    int
	styles   Styles
}

// NewStatusBar creates a status bar.
func NewStatusBar(styles Styles) StatusBar {
	return StatusBar{
		sections: make(map[string]string),
		styles:   styles,
	}
}

// SetRegistry updates the registered section definitions.
// Sections are sorted once here so renderSide does not need to sort on every render.
func (s *StatusBar) SetRegistry(sections []ext.StatusSection) {
	s.registry = make([]ext.StatusSection, len(sections))
	copy(s.registry, sections)
	sort.Slice(s.registry, func(i, j int) bool {
		if s.registry[i].Side != s.registry[j].Side {
			return s.registry[i].Side < s.registry[j].Side
		}
		return s.registry[i].Order < s.registry[j].Order
	})
}

// Set updates a named status section's display value.
// Pass empty string to clear the section.
func (s *StatusBar) Set(key, value string) {
	if value == "" {
		delete(s.sections, key)
	} else {
		s.sections[key] = value
	}
}

// SetThinking updates the thinking indicator.
func (s *StatusBar) SetThinking(on bool) { s.thinking = on }

// SetSpinning updates the spinner state.
func (s *StatusBar) SetSpinning(on bool) { s.spinning = on }

// SetWidth updates the available width.
func (s *StatusBar) SetWidth(w int) { s.width = w }

// View renders the status bar.
func (s StatusBar) View() string {
	left := s.renderSide(ext.StatusLeft)
	right := s.renderSide(ext.StatusRight)

	// Prepend thinking/spinning indicator to right side
	var indicator string
	if s.thinking {
		indicator = s.styles.Spinner.Render("thinking...")
	} else if s.spinning {
		indicator = s.styles.Spinner.Render("...")
	}
	if indicator != "" {
		if right != "" {
			right = indicator + " " + right
		} else {
			right = indicator
		}
	}

	gap := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return s.styles.Footer.Render(left + strings.Repeat(" ", gap) + right)
}

// renderSide collects and renders all sections for a given side.
// Registry is pre-sorted by SetRegistry, so no sort is needed here.
func (s StatusBar) renderSide(side ext.StatusSide) string {
	var parts []string
	for _, sec := range s.registry {
		if sec.Side != side {
			continue
		}
		val, ok := s.sections[sec.Key]
		if !ok || val == "" {
			continue
		}
		parts = append(parts, val)
	}

	sep := " "
	if side == ext.StatusLeft {
		sep = s.styles.Muted.Render(" | ")
	}
	return strings.Join(parts, sep)
}
