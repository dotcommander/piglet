package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
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

	// Event channel (mirrors shell.EventChannel for polling)
	eventCh <-chan core.Event

	// Background event channel (mirrors shell.BgEventChannel for polling)
	bgEventCh <-chan core.Event

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

	// Mouse mode: when true, bubbletea captures mouse (scroll via wheel);
	// when false (default), native terminal text selection works.
	mouseEnabled bool

	// Rendered message cache — parallel to m.messages, invalidated on width change
	msgCache []string

	// Extension widgets — keyed, last-write-wins per key, max 5 lines each
	widgets map[string]widgetState
}

// widgetState holds the content and placement of a single extension widget.
type widgetState struct {
	Placement string // "above-input" or "below-status"
	Content   string
}

// New creates a TUI model.
func New(cfg Config) Model {
	styles := NewStyles(cfg.Theme)

	var commands []string
	if cfg.Shell != nil {
		commands = cfg.Shell.CommandNames()
	} else {
		commands = commandNames(cfg.App)
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
	vp.MouseWheelDelta = 1
	vp.Style = lipgloss.NewStyle()

	m := Model{
		cfg:          cfg,
		shell:        cfg.Shell,
		styles:       styles,
		input:        NewInputModel(styles, commands),
		viewport:     vp,
		status:       status,
		msgView:      NewMessageView(styles, 80),
		overlays:     NewOverlayModel(styles),
		spinner:      sp,
		streamText:   &strings.Builder{},
		streamThink:  &strings.Builder{},
		focused:      true,
		followOutput: true,
		widgets:      make(map[string]widgetState),
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

// handleSpinnerTick advances the spinner animation during streaming.
func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.streaming {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.status.SetSpinnerView(m.spinner.View() + " " + m.spinnerVerb)
		return m, cmd
	}
	return m, nil
}

// handleEventsBatch processes a batch of agent events, emitting a single
// pollEvents at the end rather than one per event.
func (m Model) handleEventsBatch(msg eventsBatchMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var model tea.Model = m
	for _, evt := range msg.events {
		var cmd tea.Cmd
		model, cmd = m.handleEvent(evt, true)
		m = model.(Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Single pollEvents for the entire batch
	if m.eventCh != nil && m.streaming {
		cmds = append(cmds, pollEvents(m.eventCh))
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// handleModalSelect fires the picker callback and drains shell actions.
func (m Model) handleModalSelect(msg ModalSelectMsg) (tea.Model, tea.Cmd) {
	if m.pickerCallback != nil {
		cb := m.pickerCallback
		m.pickerCallback = nil
		cb(ext.PickerItem{
			ID:    msg.Item.ID,
			Label: msg.Item.Label,
			Desc:  msg.Item.Desc,
		})
		if m.shell != nil {
			m.shell.DrainActions()
		}
		if bgCmd := m.applyShellNotifications(); bgCmd != nil {
			return m, bgCmd
		}
	}
	return m, nil
}

// handleNotifyTick decrements the notification countdown and clears when done.
func (m Model) handleNotifyTick() (tea.Model, tea.Cmd) {
	if m.notificationTimer > 0 {
		m.notificationTimer--
		if m.notificationTimer > 0 {
			return m, notifyTick()
		}
		m.notification = ""
	}
	return m, nil
}

// handleAsyncAction re-enqueues the result from an ActionRunAsync and applies it.
func (m Model) handleAsyncAction(msg asyncActionMsg) (tea.Model, tea.Cmd) {
	if m.shell != nil && msg.action != nil {
		m.shell.EnqueueResult(msg.action)
		m.shell.DrainActions()
		if bgCmd := m.applyShellNotifications(); bgCmd != nil {
			return m, bgCmd
		}
	}
	return m, nil
}

// handleAgentReady wires the fully-initialized agent into the model after
// background setup completes.
func (m Model) handleAgentReady(msg AgentReadyMsg) (tea.Model, tea.Cmd) {
	m.cfg.Agent = msg.Agent
	m.cfg.SetupFn = nil
	if m.shell != nil {
		m.shell.SetAgent(msg.Agent)
	}
	m.status.Set(ext.StatusKeyApp, m.styles.Muted.Render("piglet"))
	if m.cfg.App != nil {
		m.status.SetRegistry(m.cfg.App.StatusSections())
		m.input.SetCommands(m.shell.CommandNames())
	}
	return m, m.notifyAndTick("Extensions loaded")
}

// handleKeyPress processes keyboard input. Returns handled=true if the key
// was consumed (modal, global shortcut, submit). When handled=false the
// caller should let the input textarea handle the key.
func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.modal.Visible() {
		var cmd tea.Cmd
		m.modal, cmd = m.modal.Update(msg)
		return m, cmd, true
	}

	if m.overlays.Visible() {
		switch {
		case msg.Code == tea.KeyEscape:
			m.overlays.DismissTop()
			return m, nil, true
		case msg.Code == tea.KeyUp:
			m.overlays.ScrollUp()
			return m, nil, true
		case msg.Code == tea.KeyDown:
			m.overlays.ScrollDown()
			return m, nil, true
		}
		return m, nil, true // consume all other keys while overlay visible
	}

	switch {
	case msg.Code == 'c' && msg.Mod.Contains(tea.ModCtrl):
		if m.streaming && m.shell != nil {
			m.shell.Abort()
			m.stopStreaming()
			return m, nil, true
		}
		if m.shell != nil {
			m.shell.StopBackground()
		}
		m.quitting = true
		return m, tea.Quit, true

	case msg.Code == tea.KeyPgUp, msg.Code == tea.KeyPgDown:
		m.viewport, _ = m.viewport.Update(msg)
		m.followOutput = m.viewport.AtBottom()
		return m, nil, true

	case msg.Code == tea.KeyEnter && !msg.Mod.Contains(tea.ModAlt):
		result, cmd := m.handleSubmit()
		return result, cmd, true

	case msg.Code == 'm' && msg.Mod.Contains(tea.ModCtrl):
		m.mouseEnabled = !m.mouseEnabled
		var cmd tea.Cmd
		if m.mouseEnabled {
			m.status.Set(ext.StatusKeyMouse, m.styles.Muted.Render("mouse"))
			cmd = m.notifyAndTick("mouse mode ON — scroll with wheel, Ctrl+M to toggle off")
		} else {
			m.status.Set(ext.StatusKeyMouse, "")
			cmd = m.notifyAndTick("mouse mode OFF — native text selection restored")
		}
		return m, cmd, true

	case msg.Code == 'z' && msg.Mod.Contains(tea.ModCtrl):
		return m, tea.Suspend, true

	default:
		if result, cmd, handled := m.runShortcut(msg); handled {
			return result, cmd, true
		}
	}

	return m, nil, false
}

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

// ---------------------------------------------------------------------------
// Internal
// ---------------------------------------------------------------------------

// stopStreaming resets all streaming-related state and transitions to idle.
func (m *Model) stopStreaming() {
	m.streaming = false
	m.activeTool = ""
	m.spinnerVerb = ""
	m.status.SetSpinnerView("")
	m.streamCache = streamCache{}
	m.refreshAndFollow()
	if m.cfg.App != nil {
		m.cfg.App.SignalIdle()
	}
}

func (m *Model) layout() {
	statusH := 1
	inputH := 5 // border + 3 lines + border
	vpH := m.height - statusH - inputH - 2
	if vpH < 3 {
		vpH = 3
	}

	wasAtBottom := m.followOutput
	m.viewport.SetWidth(m.width - 2)
	m.viewport.SetHeight(vpH)
	m.refreshViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}

	m.input.SetWidth(m.width)
	m.status.SetWidth(m.width - 2) // subtract App padding
	m.msgView.SetWidth(m.width - 2)
	m.msgCache = nil
	m.modal.SetSize(m.width, m.height)
	m.overlays.SetSize(m.width, m.height)
}

func (m *Model) showNotification(text string) {
	m.notification = text
	m.notificationLevel = ""
	m.notificationTimer = notifyDuration
}

// notificationStyle returns the lipgloss style for the current notification level.
func (m Model) notificationStyle() lipgloss.Style {
	switch m.notificationLevel {
	case "warn":
		return m.styles.Warning
	case "error":
		return m.styles.Error
	default:
		return m.styles.Muted
	}
}

// appendDisplayMessage adds a message to the TUI display list.
// Persistence is handled by shell.ProcessEvent.
func (m *Model) appendDisplayMessage(msg core.Message) {
	m.messages = append(m.messages, msg)
}

// notifyAndTick shows a notification and returns the tick command to dismiss it.
func (m *Model) notifyAndTick(text string) tea.Cmd {
	m.showNotification(text)
	return notifyTick()
}

func notifyTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return notifyTickMsg{}
	})
}
