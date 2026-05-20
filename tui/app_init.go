package tui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/tool"
)

// New creates a TUI model.
func New(cfg Config) Model {
	styles := NewStyles(cfg.Theme)

	var commands []CommandSuggestion
	if cfg.Shell != nil {
		for _, d := range cfg.Shell.CommandInfos() {
			commands = append(commands, CommandSuggestion{Name: d.Name, Description: d.Description})
		}
	} else {
		commands = commandSuggestions(cfg.App)
	}

	status := NewStatusBar(styles)
	if cfg.App != nil {
		status.SetRegistry(cfg.App.StatusSections())
	}
	if cfg.SetupFn != nil {
		status.Set(ext.StatusKeyApp, styles.Muted.Render("piglet (loading...)"))
	} else {
		status.Set(ext.StatusKeyApp, styles.Muted.Render("piglet"))
	}
	status.Set(ext.StatusKeyModel, styles.Muted.Render(cfg.Model.DisplayName()))

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = styles.Spinner

	vp := viewport.New()
	vp.MouseWheelDelta = 3
	vp.Style = lipgloss.NewStyle()

	mouseOn := true
	if cfg.Settings != nil {
		mouseOn = cfg.Settings.MouseCaptureEnabled()
	}
	m := Model{
		cfg:           cfg,
		shell:         cfg.Shell,
		styles:        styles,
		input:         NewInputModel(styles, commands),
		viewport:      vp,
		status:        status,
		msgView:       NewMessageView(styles, 80, cfg.Theme.GlamourStyle),
		overlays:      NewOverlayModel(styles),
		spinner:       sp,
		streamText:    &strings.Builder{},
		streamThink:   &strings.Builder{},
		focused:       true,
		followOutput:  true,
		mouseEnabled:  mouseOn,
		widgets:       make(map[string]widgetState),
		diffMeta:      make(map[string]tool.DiffMeta),
		expandedTools: make(map[string]bool),
		bashTailCh:    subscribeBashTail(cfg.App),
	}
	// Seed the status section. Empty text clears; non-empty renders.
	if mouseOn {
		status.Set(ext.StatusKeyMouse, styles.Muted.Render("mouse"))
	}
	if hp, err := config.HistoryPath(); err == nil {
		m.input.LoadHistory(hp)
	}
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.input.textarea.Focus()}
	if m.cfg.SetupFn != nil {
		fn := m.cfg.SetupFn
		cmds = append(cmds, func() tea.Msg { return fn() })
	}
	if c := drainBashTail(m.bashTailCh); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}
