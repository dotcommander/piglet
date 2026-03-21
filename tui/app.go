package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/session"
)

// Config configures the TUI app.
type Config struct {
	Agent   *core.Agent
	Session *session.Session
	Model   core.Model
	Models  []core.Model // available models from registry
	SessDir string       // session directory path
	Theme   Theme
	App     *ext.App // extension API surface
}

// eventMsg wraps agent events for the Bubble Tea message loop.
type eventMsg struct{ event core.Event }

// tickMsg drives periodic refresh during streaming.
type tickMsg struct{}

// cmdResult captures what a command handler wants the TUI to do.
// Callbacks write here; runCommand reads after handler returns.
type cmdResult struct {
	message     string
	quit        bool
	modelName   string
	pickerTitle string
	pickerItems []ModalItem
	pickerCB    func(ext.PickerItem)
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

	// State
	messages       []core.Message
	streaming      bool
	streamText     string
	streamThink    string
	activeTool     string
	totalIn        int
	totalOut       int
	quitting       bool
	pickerCallback func(ext.PickerItem)

	// Shared command result (pointer so callbacks can write to it)
	pendingResult *cmdResult

	// Event channel
	eventCh <-chan core.Event
}

// New creates a TUI model.
func New(cfg Config) Model {
	styles := NewStyles(cfg.Theme)
	commands := commandNames(cfg.App)

	status := NewStatusBar(styles)
	status.SetModel(cfg.Model.DisplayName())

	return Model{
		cfg:     cfg,
		styles:  styles,
		input:   NewInputModel(styles, commands),
		status:  status,
		msgView: NewMessageView(styles, 80),
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
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case msg.Code == tea.KeyEnter && !msg.Mod.Contains(tea.ModAlt):
			return m.handleSubmit()

		default:
			// Dispatch registered keyboard shortcuts
			if result, handled := m.runShortcut(msg); handled {
				return result, nil
			}
		}

	case eventMsg:
		return m.handleEvent(msg.event)

	case tickMsg:
		if m.streaming {
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
			m.applyPendingResult()
		}
		return m, nil

	case ModalCloseMsg:
		m.pickerCallback = nil
		return m, nil
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

	// Messages viewport — bottom-aligned like Claude Code
	content := m.renderMessages()
	contentLines := strings.Count(content, "\n")
	vpHeight := m.viewport.Height()
	if contentLines < vpHeight {
		content = strings.Repeat("\n", vpHeight-contentLines) + content
	}
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
	sections = append(sections, m.viewport.View())

	// Input
	sections = append(sections, m.input.View())

	// Status bar
	sections = append(sections, m.status.View())

	// Modal overlay
	if m.modal.Visible() {
		return tea.NewView(m.modal.View())
	}

	return tea.NewView(m.styles.App.Render(strings.Join(sections, "\n")))
}

// Run starts the TUI.
func Run(cfg Config) error {
	m := New(cfg)
	p := tea.NewProgram(m,
		tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithFilter(filterTerminalResponses),
	)
	_, err := p.Run()
	return err
}

// filterTerminalResponses drops unknown terminal response sequences
// (OSC, CSI, DCS, etc.) that bubbletea's ultraviolet parser doesn't
// recognize. Without this, they leak through as raw events.
func filterTerminalResponses(_ tea.Model, msg tea.Msg) tea.Msg {
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

	m.viewport = viewport.New(viewport.WithWidth(m.width-2), viewport.WithHeight(vpH))
	m.viewport.Style = lipgloss.NewStyle()

	m.input.SetWidth(m.width)
	m.status.SetWidth(m.width)
	m.msgView.SetWidth(m.width - 2)
	m.modal.SetSize(m.width, m.height)
}

func (m Model) renderMessages() string {
	var b strings.Builder

	for _, msg := range m.messages {
		b.WriteString(m.msgView.RenderMessage(msg))
		b.WriteByte('\n')
	}

	// Streaming content
	if m.streaming {
		b.WriteString(m.msgView.RenderStreaming(m.streamText, m.streamThink))
	}

	// Active tool indicator
	if m.activeTool != "" {
		b.WriteString(m.styles.ToolName.Render(fmt.Sprintf("running: %s", m.activeTool)))
		b.WriteByte('\n')
	}

	return b.String()
}

func (m Model) handleSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.Reset()

	// Slash command?
	if strings.HasPrefix(text, "/") {
		parts := strings.Fields(text)
		cmd := strings.TrimPrefix(parts[0], "/")
		args := ""
		if len(parts) > 1 {
			args = strings.Join(parts[1:], " ")
		}
		return m.runCommand(cmd, args)
	}

	// Send to agent
	userMsg := &core.UserMessage{
		Content:   text,
		Timestamp: time.Now(),
	}
	m.messages = append(m.messages, userMsg)

	// Persist user message
	if m.cfg.Session != nil {
		_ = m.cfg.Session.Append(userMsg)
	}

	// Start agent
	ch := m.cfg.Agent.Start(context.Background(), text)
	m.eventCh = ch
	m.streaming = true

	return m, tea.Batch(pollEvents(ch), tickCmd())
}

// bindApp wires ext.App callbacks to write to m.pendingResult.
// Called before each command handler invocation.
func (m *Model) bindApp() {
	if m.cfg.App == nil {
		return
	}
	r := &cmdResult{}
	m.pendingResult = r

	m.cfg.App.Bind(m.cfg.Agent,
		ext.WithShowMessage(func(text string) {
			r.message = text
		}),
		ext.WithRequestQuit(func() {
			r.quit = true
		}),
		ext.WithShowPicker(func(title string, items []ext.PickerItem, onSelect func(ext.PickerItem)) {
			r.pickerTitle = title
			r.pickerItems = make([]ModalItem, len(items))
			for i, item := range items {
				r.pickerItems[i] = ModalItem{ID: item.ID, Label: item.Label, Desc: item.Desc}
			}
			r.pickerCB = onSelect
		}),
		ext.WithStatus(func(key, text string) {
			if key == "model" {
				r.modelName = text
			}
		}),
	)
}

// applyPendingResult reads the pending result and applies it to the model.
func (m *Model) applyPendingResult() {
	r := m.pendingResult
	if r == nil {
		return
	}
	m.pendingResult = nil

	if r.message != "" {
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{core.TextContent{Text: r.message}},
		})
	}
	if r.modelName != "" {
		m.status.SetModel(r.modelName)
		m.cfg.Model = findModel(m.cfg.Models, r.modelName)
	}
	if r.pickerTitle != "" {
		m.modal = NewModalModel(r.pickerTitle, r.pickerItems, m.styles)
		m.modal.SetSize(m.width, m.height)
		m.modal.Show()
		m.pickerCallback = r.pickerCB
	}
	if r.quit {
		m.quitting = true
	}
}

// runCommand dispatches a slash command to the registered handler.
func (m Model) runCommand(name, args string) (tea.Model, tea.Cmd) {
	if m.cfg.App == nil {
		return m, nil
	}

	// Alias
	if name == "exit" {
		name = "quit"
	}

	cmds := m.cfg.App.Commands()
	cmd, ok := cmds[name]
	if !ok {
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{core.TextContent{Text: "Unknown command: /" + name}},
		})
		return m, nil
	}

	// Bind callbacks, run handler, apply results
	m.bindApp()

	// Special handling for /clear: clear messages before handler runs
	if name == "clear" {
		m.messages = nil
		m.cfg.Agent.SetMessages(nil)
	}

	if err := cmd.Handler(args, m.cfg.App); err != nil {
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{core.TextContent{Text: "Command error: " + err.Error()}},
		})
		return m, nil
	}

	m.applyPendingResult()

	if m.quitting {
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) handleEvent(evt core.Event) (tea.Model, tea.Cmd) {
	switch e := evt.(type) {
	case core.EventStreamDelta:
		if e.Kind == "text" {
			m.streamText += e.Delta
		} else if e.Kind == "thinking" {
			m.streamThink += e.Delta
		}

	case core.EventStreamDone:
		m.streamText = ""
		m.streamThink = ""

	case core.EventToolStart:
		m.activeTool = e.ToolName

	case core.EventToolEnd:
		m.activeTool = ""

	case core.EventTurnEnd:
		if e.Assistant != nil {
			m.messages = append(m.messages, e.Assistant)
			m.totalIn += e.Assistant.Usage.InputTokens
			m.totalOut += e.Assistant.Usage.OutputTokens
			m.status.SetTokens(m.totalIn, m.totalOut)

			if m.cfg.Session != nil {
				_ = m.cfg.Session.Append(e.Assistant)
			}
		}
		for _, tr := range e.ToolResults {
			m.messages = append(m.messages, tr)
			if m.cfg.Session != nil {
				_ = m.cfg.Session.Append(tr)
			}
		}

	case core.EventAgentEnd:
		m.streaming = false
		m.activeTool = ""

	case core.EventMaxTurns:
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{
				core.TextContent{Text: fmt.Sprintf("Stopped: max turns reached (%d)", e.Max)},
			},
		})

	case core.EventRetry:
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{
				core.TextContent{Text: fmt.Sprintf("Retrying (%d/%d)...", e.Attempt, e.Max)},
			},
		})
	}

	// Continue polling
	if m.eventCh != nil && m.streaming {
		return m, pollEvents(m.eventCh)
	}
	return m, nil
}

// pollEvents reads the next event from the agent channel.
func pollEvents(ch <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			return eventMsg{event: core.EventAgentEnd{}}
		}
		return eventMsg{event: evt}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// commandNames returns sorted slash command names from the ext.App.
func commandNames(app *ext.App) []string {
	if app == nil {
		return nil
	}
	cmds := app.Commands()
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// runShortcut checks if the key matches a registered shortcut and runs it.
func (m *Model) runShortcut(msg tea.KeyPressMsg) (tea.Model, bool) {
	if m.cfg.App == nil {
		return m, false
	}

	key := keyString(msg)
	if key == "" {
		return m, false
	}

	shortcuts := m.cfg.App.Shortcuts()
	sc, ok := shortcuts[key]
	if !ok {
		return m, false
	}

	m.bindApp()
	_ = sc.Handler(m.cfg.App)
	m.applyPendingResult()

	return m, true
}

// keyString converts a KeyPressMsg to the shortcut key format (e.g. "ctrl+p").
func keyString(msg tea.KeyPressMsg) string {
	if !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt) {
		return ""
	}
	var parts []string
	if msg.Mod.Contains(tea.ModCtrl) {
		parts = append(parts, "ctrl")
	}
	if msg.Mod.Contains(tea.ModAlt) {
		parts = append(parts, "alt")
	}
	if msg.Mod.Contains(tea.ModShift) {
		parts = append(parts, "shift")
	}
	if msg.Code >= 'a' && msg.Code <= 'z' {
		parts = append(parts, string(msg.Code))
	} else {
		return ""
	}
	return strings.Join(parts, "+")
}

// findModel looks up a model by "provider/name" or plain name.
func findModel(models []core.Model, query string) core.Model {
	for _, m := range models {
		if m.DisplayName() == query || m.Name == query {
			return m
		}
	}
	return core.Model{}
}
