package tui

import (
	"fmt"
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
	streamText      string
	streamThink     string
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
		// Global shortcuts
		if m.modal.Visible() {
			var cmd tea.Cmd
			m.modal, cmd = m.modal.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		switch {
		case msg.Code == 'c' && msg.Mod.Contains(tea.ModCtrl):
			if m.streaming {
				m.cfg.Agent.Stop()
				m.streaming = false
				m.spinnerVerb = ""
				m.status.SetSpinnerView("")
				m.streamCache = StreamCache{}
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case msg.Code == tea.KeyPgUp, msg.Code == tea.KeyPgDown:
			m.viewport, _ = m.viewport.Update(msg)
			m.followOutput = m.viewport.AtBottom()
			return m, nil

		case msg.Code == tea.KeyEnter && !msg.Mod.Contains(tea.ModAlt):
			return m.handleSubmit()

		case msg.Code == 'z' && msg.Mod.Contains(tea.ModCtrl):
			return m, tea.Suspend

		default:
			// Dispatch registered keyboard shortcuts
			if result, cmd, handled := m.runShortcut(msg); handled {
				return result, cmd
			}
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
			m.refreshViewport()
			if m.followOutput {
				m.viewport.GotoBottom()
			}
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
			m.showNotification("editor: " + msg.err.Error())
			cmds = append(cmds, notifyTick())
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

// View implements tea.Model.
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var sections []string

	// Messages viewport
	sections = append(sections, m.viewport.View())

	// Toast notification (transient, above input)
	if m.notification != "" {
		sections = append(sections, m.styles.Muted.Render(" "+m.notification+" "))
	}

	// Input
	sections = append(sections, m.input.View())

	// Status bar
	sections = append(sections, m.status.View())

	// Modal overlay
	if m.modal.Visible() {
		return tea.NewView(m.modal.View())
	}

	v := tea.NewView(m.styles.App.Render(strings.Join(sections, "\n")))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = m.windowTitle()
	return v
}

// windowTitle returns the terminal window title.
func (m Model) windowTitle() string {
	title := "piglet"
	if m.cfg.Session != nil {
		if name := m.cfg.Session.Meta().Title; name != "" {
			title += " — " + name
		}
	}
	return title
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

// refreshViewport updates the viewport content from messages without changing scroll position.
func (m *Model) refreshViewport() {
	content := "\n" + m.renderMessages()
	contentLines := strings.Count(content, "\n")
	vpHeight := m.viewport.Height()
	if contentLines < vpHeight {
		content = strings.Repeat("\n", vpHeight-contentLines) + content
	}
	m.viewport.SetContent(content)
}

func (m *Model) showNotification(text string) {
	m.notification = text
	m.notificationTimer = notifyDuration
}

func notifyTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return notifyTickMsg{}
	})
}

func (m Model) renderMessages() string {
	var b strings.Builder

	for _, msg := range m.messages {
		b.WriteString(m.msgView.RenderMessage(msg))
		b.WriteString("\n\n")
	}

	// Streaming content
	if m.streaming {
		b.WriteString(m.msgView.RenderStreaming(m.streamText, m.streamThink, &m.streamCache))
	}

	// Active tool indicator
	if m.activeTool != "" {
		b.WriteString(m.styles.ToolName.Render(fmt.Sprintf("running: %s", m.activeTool)))
		b.WriteByte('\n')
	}

	return b.String()
}

func formatTokens(in, out, cacheRead int) string {
	if cacheRead > 0 {
		return fmt.Sprintf("%dk/%dk (cached:%dk)", in/1000, out/1000, cacheRead/1000)
	}
	return fmt.Sprintf("%dk/%dk", in/1000, out/1000)
}

func formatCost(c float64) string {
	if c < 0.01 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", c)
}

// formatImageSize formats a byte count as a human-readable string.
func formatImageSize(bytes int) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
