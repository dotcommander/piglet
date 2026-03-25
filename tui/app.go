package tui

import (
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
)

// Config configures the TUI app.
type Config struct {
	Agent    *core.Agent
	Session  *session.Session
	Model    core.Model
	Models   []core.Model // available models from registry
	SessDir  string       // session directory path
	Theme    Theme
	App      *ext.App         // extension API surface
	Settings *config.Settings // user settings (nil-safe)
}

// eventMsg wraps agent events for the Bubble Tea message loop.
type eventMsg struct{ event core.Event }

// tickMsg drives periodic refresh during streaming.
type tickMsg struct{}

// bgEventMsg wraps background agent events for the Bubble Tea message loop.
type bgEventMsg struct{ event core.Event }

// asyncActionMsg carries the result of an ActionRunAsync completion.
type asyncActionMsg struct{ action ext.Action }

// execDoneMsg signals that an external process (e.g., $EDITOR) has finished.
type execDoneMsg struct{ err error }

// notifyTickMsg decrements the notification timer.
type notifyTickMsg struct{}

// notifyDuration is how many ticks a toast notification stays visible.
const notifyDuration = 15 // ~3 seconds at 200ms tick

// bgStartResult is set by the background agent callback when it starts polling.
type bgStartResult struct {
	ch <-chan core.Event
}

// Model is the Bubble Tea model for the TUI.
type Model struct {
	cfg Config

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

	// Background start channel (set by bind callback, read after command)
	pendingBgStart *bgStartResult

	// Event channel
	eventCh <-chan core.Event

	// Background agent state
	bgAgent   *core.Agent
	bgEventCh <-chan core.Event
	bgTask    string
	bgResult  *strings.Builder

	// Streaming glamour cache
	streamCache StreamCache

	// Toast notification (transient, not in conversation history)
	notification      string
	notificationTimer int // ticks remaining (0 = hidden)

	// Pending image attachment for next message
	pendingImage *core.ImageContent

	// Terminal focus state
	focused bool

	// Auto-scroll: follow new output unless user scrolled up
	followOutput bool
}

// New creates a TUI model.
func New(cfg Config) Model {
	styles := NewStyles(cfg.Theme)
	commands := commandNames(cfg.App)

	status := NewStatusBar(styles)
	if cfg.App != nil {
		status.SetRegistry(cfg.App.StatusSections())
	}
	status.Set(ext.StatusKeyApp, styles.Muted.Render("piglet"))
	status.Set(ext.StatusKeyModel, styles.Muted.Render(cfg.Model.DisplayName()))

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = styles.Spinner

	return Model{
		cfg:          cfg,
		styles:       styles,
		input:        NewInputModel(styles, commands),
		status:       status,
		msgView:      NewMessageView(styles, 80),
		spinner:      sp,
		streamText:   &strings.Builder{},
		streamThink:  &strings.Builder{},
		bgResult:     &strings.Builder{},
		focused:      true,
		followOutput: true,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return m.input.textarea.Focus()
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
		if m.streaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.status.SetSpinnerView(m.spinner.View() + " " + m.spinnerVerb)
			return m, cmd
		}
		return m, nil

	case tea.MouseWheelMsg:
		m.viewport, _ = m.viewport.Update(msg)
		m.followOutput = m.viewport.AtBottom()
		return m, nil

	case eventMsg:
		return m.handleEvent(msg.event)

	case tickMsg:
		if m.streaming {
			m.refreshAndFollow()
			cmds = append(cmds, tickCmd())
		}

	case ModalSelectMsg:
		if m.pickerCallback != nil {
			cb := m.pickerCallback
			m.pickerCallback = nil
			// Set up a fresh result for the callback to write into
			m.bindApp()
			cb(ext.PickerItem{
				ID:    msg.Item.ID,
				Label: msg.Item.Label,
				Desc:  msg.Item.Desc,
			})
			bgCmd := m.applyActions()
			if bgCmd != nil {
				return m, bgCmd
			}
		}
		return m, nil

	case ModalCloseMsg:
		m.pickerCallback = nil
		return m, nil

	case bgEventMsg:
		return m.handleBgEvent(msg.event)

	case notifyTickMsg:
		if m.notificationTimer > 0 {
			m.notificationTimer--
			if m.notificationTimer > 0 {
				cmds = append(cmds, notifyTick())
			} else {
				m.notification = ""
			}
		}
		return m, tea.Batch(cmds...)

	case asyncActionMsg:
		// Re-enqueue the result action from an ActionRunAsync and apply it
		if m.cfg.App != nil && msg.action != nil {
			m.cfg.App.EnqueueAction(msg.action)
			bgCmd := m.applyActions()
			if bgCmd != nil {
				return m, bgCmd
			}
		}
		return m, nil

	case execDoneMsg:
		if msg.err != nil {
			cmds = append(cmds, m.notifyAndTick("editor: "+msg.err.Error()))
		}
		return m, tea.Batch(cmds...)

	}

	// Update input
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	if inputCmd != nil {
		cmds = append(cmds, inputCmd)
	}

	return m, tea.Batch(cmds...)
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

	switch {
	case msg.Code == 'c' && msg.Mod.Contains(tea.ModCtrl):
		if m.streaming {
			m.cfg.Agent.Stop()
			m.streaming = false
			m.spinnerVerb = ""
			m.status.SetSpinnerView("")
			m.streamCache = StreamCache{}
			return m, nil, true
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
func Run(cfg Config) error {
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

func (m *Model) layout() {
	statusH := 1
	inputH := 5 // border + 3 lines + border
	vpH := m.height - statusH - inputH - 2
	if vpH < 3 {
		vpH = 3
	}

	wasAtBottom := m.followOutput
	m.viewport = viewport.New(viewport.WithWidth(m.width-2), viewport.WithHeight(vpH))
	m.viewport.MouseWheelDelta = 1
	m.viewport.Style = lipgloss.NewStyle()
	m.refreshViewport()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}

	m.input.SetWidth(m.width)
	m.status.SetWidth(m.width - 2) // subtract App padding
	m.msgView.SetWidth(m.width - 2)
	m.modal.SetSize(m.width, m.height)
}

func (m *Model) showNotification(text string) {
	m.notification = text
	m.notificationTimer = notifyDuration
}

// appendMessage adds a message to conversation history and persists to session.
func (m *Model) appendMessage(msg core.Message) {
	m.messages = append(m.messages, msg)
	if m.cfg.Session != nil {
		_ = m.cfg.Session.Append(msg)
	}
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

