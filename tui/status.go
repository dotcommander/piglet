package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// StatusBar renders the footer status bar.
type StatusBar struct {
	model    string
	tokens   string
	thinking bool
	spinning bool
	width    int
	styles   Styles
}

// NewStatusBar creates a status bar.
func NewStatusBar(styles Styles) StatusBar {
	return StatusBar{styles: styles}
}

// SetModel updates the displayed model name.
func (s *StatusBar) SetModel(name string) { s.model = name }

// SetTokens updates the token usage display.
func (s *StatusBar) SetTokens(input, output int) {
	s.tokens = fmt.Sprintf("%dk/%dk", input/1000, output/1000)
}

// SetThinking updates the thinking indicator.
func (s *StatusBar) SetThinking(on bool) { s.thinking = on }

// SetSpinning updates the spinner state.
func (s *StatusBar) SetSpinning(on bool) { s.spinning = on }

// SetWidth updates the available width.
func (s *StatusBar) SetWidth(w int) { s.width = w }

// View renders the status bar.
func (s StatusBar) View() string {
	left := s.styles.Muted.Render("piglet")
	if s.model != "" {
		left += s.styles.Muted.Render(" | " + s.model)
	}

	var right string
	if s.thinking {
		right = s.styles.Spinner.Render("thinking...")
	} else if s.spinning {
		right = s.styles.Spinner.Render("...")
	}
	if s.tokens != "" {
		if right != "" {
			right += " "
		}
		right += s.styles.Muted.Render(s.tokens)
	}

	gap := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return s.styles.Footer.Render(left + strings.Repeat(" ", gap) + right)
}
