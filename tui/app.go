package tui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/session"
	"github.com/dotcommander/piglet/shell"
)

// Config configures the TUI app.
type Config struct {
	Shell    *shell.Shell
	Agent    *core.Agent
	Session  *session.Session
	Model    core.Model
	Models   []core.Model // available models from registry
	SessDir  string       // session directory path
	Theme    Theme
	App      *ext.App         // extension API surface
	Settings *config.Settings // user settings (nil-safe)

	// SetupFn runs as a background Cmd from Init(). It performs heavy startup
	// work (loading extensions, building agent) and returns an AgentReadyMsg.
	SetupFn func() AgentReadyMsg
}

// eventMsg wraps agent events for the Bubble Tea message loop.
type eventMsg struct{ event core.Event }

// tickMsg drives periodic refresh during streaming.
type tickMsg struct{}

// bgEventMsg wraps background agent events for the Bubble Tea message loop.
type bgEventMsg struct{ event core.Event }

// eventsBatchMsg carries a batch of agent events for efficient processing.
type eventsBatchMsg struct{ events []core.Event }

// asyncActionMsg carries the result of an ActionRunAsync completion.
type asyncActionMsg struct{ action ext.Action }

// execDoneMsg signals that an external process (e.g., $EDITOR) has finished.
type execDoneMsg struct{ err error }

// AgentReadyMsg signals background setup is complete and the agent is ready.
type AgentReadyMsg struct {
	Agent *core.Agent
}

// notifyTickMsg decrements the notification timer.
type notifyTickMsg struct{}

// notifyDuration is how many ticks a toast notification stays visible.
const notifyDuration = 15 // ~3 seconds at 200ms tick

// Model is the Bubble Tea model for the TUI.
type Model struct {
	cfg   Config
	shell *shell.Shell

	// Layout
	width  int
	height int

	// Components
	styles   Styles
	input    InputModel
	viewport viewport.Model
	status   StatusBar
	msgView  MessageView
	modal    ModalModel
	overlays OverlayModel
	spinner  spinner.Model

	// State
	messages        []core.Message
	streaming       bool
	streamText      *strings.Builder
	streamThink     *strings.Builder
	activeTool      string
	spinnerVerb     string
	totalIn         int
	totalOut        int
	totalCost       float64
	totalCacheRead  int
	totalCacheWrite int
	quitting        bool
	pickerCallback  func(ext.PickerItem)
	askUserCallback func(ext.AskUserResult)

	// Event channel (mirrors shell.EventChannel for polling)
	eventCh <-chan core.Event

	// Background event channel (mirrors shell.BgEventChannel for polling)
	bgEventCh  <-chan core.Event
	bgTaskName string

	// Streaming glamour cache
	streamCache streamCache

	// Toast notification (transient, not in conversation history)
	notification      string
	notificationLevel string // "", "info" → muted; "warn" → warning; "error" → error
	notificationTimer int    // ticks remaining (0 = hidden)

	// Pending image attachment for next message
	pendingImage *core.ImageContent

	// Terminal focus state
	focused bool

	// Auto-scroll: follow new output unless user scrolled up
	followOutput bool

	// Mouse mode: default ON — bubbletea captures mouse so wheel/trackpad
	// scrolls the viewport. Hold Shift (or Option on macOS terminals) to
	// bypass capture and use native text selection — this is standard
	// across iTerm2, Terminal.app, kitty, wezterm, Alacritty, ghostty.
	// Toggle at runtime via /mouse; persisted in config.MouseCapture.
	mouseEnabled bool

	// Rendered message cache — parallel to m.messages, invalidated on width change
	msgCache []cachedMsg

	// Extension widgets — keyed, last-write-wins per key, max 5 lines each
	widgets map[string]widgetState
}

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
		cfg:          cfg,
		shell:        cfg.Shell,
		styles:       styles,
		input:        NewInputModel(styles, commands),
		viewport:     vp,
		status:       status,
		msgView:      NewMessageView(styles, 80, cfg.Theme.GlamourStyle),
		overlays:     NewOverlayModel(styles),
		spinner:      sp,
		streamText:   &strings.Builder{},
		streamThink:  &strings.Builder{},
		focused:      true,
		followOutput: true,
		mouseEnabled: mouseOn,
		widgets:      make(map[string]widgetState),
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
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()

	case tea.KeyPressMsg:
		if result, cmd, handled := m.handleKeyPress(msg); handled {
			return result, cmd
		}

	case tea.ResumeMsg:
		return m, m.input.textarea.Focus()

	case tea.FocusMsg:
		m.focused = true
		return m, nil

	case tea.BlurMsg:
		m.focused = false
		return m, nil

	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)

	case tea.MouseWheelMsg:
		m.viewport, _ = m.viewport.Update(msg)
		m.followOutput = m.viewport.AtBottom()
		return m, nil

	case eventMsg:
		return m.handleEvent(msg.event, false)

	case eventsBatchMsg:
		return m.handleEventsBatch(msg)

	case tickMsg:
		if m.streaming {
			m.refreshAndFollow()
			cmds = append(cmds, tickCmd())
		}

	case ModalSelectMsg:
		return m.handleModalSelect(msg)

	case ModalCloseMsg:
		m.pickerCallback = nil
		return m, nil

	case ModalAskCancelMsg:
		return m.handleModalAskCancel()

	case bgEventMsg:
		return m.handleBgEvent(msg.event)

	case notifyTickMsg:
		return m.handleNotifyTick()

	case asyncActionMsg:
		return m.handleAsyncAction(msg)

	case execDoneMsg:
		if msg.err != nil {
			cmds = append(cmds, m.notifyAndTick("editor: "+msg.err.Error()))
		}
		return m, tea.Batch(cmds...)

	case AgentReadyMsg:
		return m.handleAgentReady(msg)
	}

	// Update input
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	if inputCmd != nil {
		cmds = append(cmds, inputCmd)
	}

	return m, tea.Batch(cmds...)
}
