// Package tui implements the interactive terminal UI using Bubble Tea.
package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds all colors used by the TUI.
type Theme struct {
	Primary    color.Color
	Secondary  color.Color
	Muted      color.Color
	Error      color.Color
	Success    color.Color
	Warning    color.Color
	Background color.Color
	Foreground color.Color
	Border     color.Color
}

// DefaultTheme returns the built-in color theme.
func DefaultTheme() Theme {
	return Theme{
		Primary:    lipgloss.Color("#7C3AED"),
		Secondary:  lipgloss.Color("#06B6D4"),
		Muted:      lipgloss.Color("#6B7280"),
		Error:      lipgloss.Color("#EF4444"),
		Success:    lipgloss.Color("#10B981"),
		Warning:    lipgloss.Color("#F59E0B"),
		Background: lipgloss.Color("#1E1E2E"),
		Foreground: lipgloss.Color("#CDD6F4"),
		Border:     lipgloss.Color("#45475A"),
	}
}

// Styles holds precomputed lipgloss styles derived from a Theme.
type Styles struct {
	App            lipgloss.Style
	Header         lipgloss.Style
	Footer         lipgloss.Style
	UserMsg        lipgloss.Style
	AssistantLabel lipgloss.Style
	ToolError      lipgloss.Style
	Thinking       lipgloss.Style
	Spinner        lipgloss.Style
	InputBorder    lipgloss.Style
	Muted          lipgloss.Style
	Error          lipgloss.Style
	Success        lipgloss.Style
	Warning        lipgloss.Style
	BorderColor    color.Color
}

// NewStyles creates styles from a theme.
func NewStyles(t Theme) Styles {
	return Styles{
		App: lipgloss.NewStyle().
			Padding(0, 1),
		Header: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true).
			Padding(0, 1),
		Footer: lipgloss.NewStyle().
			Foreground(t.Muted).
			Padding(0, 1),
		UserMsg: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),
		AssistantLabel: lipgloss.NewStyle().
			Foreground(t.Secondary),
		ToolError: lipgloss.NewStyle().
			Foreground(t.Error),
		Thinking: lipgloss.NewStyle().
			Foreground(t.Muted).
			Italic(true),
		Spinner: lipgloss.NewStyle().
			Foreground(t.Primary),
		InputBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.Border).
			Padding(0, 1),
		Muted: lipgloss.NewStyle().
			Foreground(t.Muted),
		Error: lipgloss.NewStyle().
			Foreground(t.Error),
		Success: lipgloss.NewStyle().
			Foreground(t.Success),
		Warning: lipgloss.NewStyle().
			Foreground(t.Warning),
		BorderColor: t.Border,
	}
}
