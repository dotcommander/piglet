package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

// Run starts the TUI.
func Run(ctx context.Context, cfg Config) error {
	m := New(cfg)
	p := tea.NewProgram(m,
		tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithFilter(messageFilter),
	)
	_, err := p.Run()
	return err
}

// messageFilter drops unwanted messages before they reach Update.
// It blocks unknown terminal response sequences and prevents quit during streaming.
func messageFilter(m tea.Model, msg tea.Msg) tea.Msg {
	if _, ok := msg.(tea.QuitMsg); ok {
		if mdl, ok := m.(Model); ok && mdl.streaming {
			return nil
		}
	}
	switch msg.(type) {
	case uv.UnknownEvent,
		uv.UnknownCsiEvent,
		uv.UnknownOscEvent,
		uv.UnknownSs3Event,
		uv.UnknownDcsEvent,
		uv.UnknownSosEvent,
		uv.UnknownPmEvent,
		uv.UnknownApcEvent:
		return nil
	}
	return msg
}
