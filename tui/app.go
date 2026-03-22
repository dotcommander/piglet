package tui

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
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

func (m Model) handleSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.PushHistory(text)
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

	// Re-follow output when user sends a message
	m.followOutput = true

	// Send to agent
	userMsg := &core.UserMessage{
		Content:   text,
		Timestamp: time.Now(),
	}
	if m.pendingImage != nil {
		userMsg.Blocks = append(userMsg.Blocks, *m.pendingImage)
		m.pendingImage = nil
		m.input.SetAttachment("")
	}
	m.messages = append(m.messages, userMsg)

	// Persist user message
	if m.cfg.Session != nil {
		_ = m.cfg.Session.Append(userMsg)
	}

	m.refreshViewport()
	m.viewport.GotoBottom()

	// Run message hooks for ephemeral turn context
	if m.cfg.App != nil {
		if injections, err := m.cfg.App.RunMessageHooks(context.Background(), text); err == nil && len(injections) > 0 {
			m.cfg.Agent.SetTurnContext(injections)
		}
	}

	// Start agent
	ch := m.cfg.Agent.Start(context.Background(), text)
	m.eventCh = ch
	m.streaming = true
	m.spinnerVerb = "thinking..."

	return m, tea.Batch(pollEvents(ch), tickCmd(), m.spinner.Tick)
}

// bindApp wires sync callbacks that need return values or direct TUI mutation.
// Fire-and-forget intents (ShowMessage, Quit, etc.) use the action queue.
func (m *Model) bindApp() {
	if m.cfg.App == nil {
		return
	}
	m.pendingBgStart = nil

	m.cfg.App.Bind(m.cfg.Agent,
		ext.WithRunBackground(func(prompt string) error {
			if m.bgAgent != nil && m.bgAgent.IsRunning() {
				return fmt.Errorf("background task already running")
			}
			tools := m.cfg.App.BackgroundSafeTools()
			bgMax := 5
			if m.cfg.Settings != nil {
				bgMax = config.IntOr(m.cfg.Settings.Agent.BgMaxTurns, 5)
			}
			m.bgAgent = core.NewAgent(core.AgentConfig{
				System:   m.cfg.Agent.System(),
				Provider: m.cfg.Agent.Provider(),
				Tools:    tools,
				MaxTurns: bgMax,
			})
			ch := m.bgAgent.Start(context.Background(), prompt)
			m.bgEventCh = ch
			m.bgTask = prompt
			m.bgResult.Reset()
			task := prompt
			if len([]rune(task)) > 20 {
				task = string([]rune(task)[:20]) + "..."
			}
			m.status.Set(ext.StatusKeyBg, m.styles.Spinner.Render("bg: "+task))
			m.pendingBgStart = &bgStartResult{ch: ch}
			return nil
		}),
		ext.WithCancelBackground(func() {
			if m.bgAgent != nil && m.bgAgent.IsRunning() {
				m.bgAgent.Stop()
			}
			m.bgAgent = nil
			m.bgEventCh = nil
			m.bgTask = ""
			m.bgResult.Reset()
			m.status.Set(ext.StatusKeyBg, "")
		}),
		ext.WithIsBackgroundRunning(func() bool {
			return m.bgAgent != nil && m.bgAgent.IsRunning()
		}),
	)
}

// applyActions drains the action queue and applies each action to the model.
// Returns a tea.Cmd if any action requires ongoing work (e.g., background agent polling).
func (m *Model) applyActions() tea.Cmd {
	if m.cfg.App == nil {
		return nil
	}

	var cmds []tea.Cmd
	for _, action := range m.cfg.App.PendingActions() {
		switch act := action.(type) {
		case ext.ActionShowMessage:
			m.showNotification(act.Text)
			cmds = append(cmds, notifyTick())
		case ext.ActionNotify:
			m.showNotification(act.Message)
			cmds = append(cmds, notifyTick())
		case ext.ActionSetStatus:
			if act.Key == ext.StatusKeyModel {
				m.cfg.Model = findModel(m.cfg.Models, act.Text)
				m.status.Set(ext.StatusKeyModel, m.styles.Muted.Render(act.Text))
			} else {
				m.status.Set(act.Key, m.styles.Muted.Render(act.Text))
			}
		case ext.ActionShowPicker:
			items := make([]ModalItem, len(act.Items))
			for i, item := range act.Items {
				items[i] = ModalItem{ID: item.ID, Label: item.Label, Desc: item.Desc}
			}
			m.modal = NewModalModel(act.Title, items, m.styles)
			m.modal.SetSize(m.width, m.height)
			m.modal.Show()
			m.pickerCallback = act.OnSelect
		case ext.ActionSwapSession:
			if s, ok := act.Session.(*session.Session); ok {
				if m.cfg.Session != nil {
					m.cfg.Session.Close()
				}
				m.cfg.Session = s
				msgs := s.Messages()
				m.messages = msgs
				m.cfg.Agent.SetMessages(msgs)
			}
		case ext.ActionAttachImage:
			if m.pendingImage != nil {
				// Toggle off — second press removes attachment
				m.pendingImage = nil
				m.input.SetAttachment("")
				m.showNotification("Image attachment removed")
				cmds = append(cmds, notifyTick())
			} else if img, ok := act.Image.(*core.ImageContent); ok {
				m.pendingImage = img
				m.input.SetAttachment("image")
				size := len(img.Data) * 3 / 4
				m.showNotification(fmt.Sprintf("Image attached (%s) — send with your next message", formatImageSize(size)))
				cmds = append(cmds, notifyTick())
			}
		case ext.ActionDetachImage:
			m.pendingImage = nil
			m.input.SetAttachment("")
			m.showNotification("Image attachment removed")
			cmds = append(cmds, notifyTick())
		case ext.ActionSetSessionTitle:
			if m.cfg.Session != nil && act.Title != "" {
				_ = m.cfg.Session.SetTitle(act.Title)
			}
		case ext.ActionRunAsync:
			fn := act.Fn
			cmds = append(cmds, func() tea.Msg {
				result := fn()
				if result != nil {
					return asyncActionMsg{action: result}
				}
				return nil
			})
		case ext.ActionExec:
			if c, ok := act.Cmd.(*exec.Cmd); ok {
				cmds = append(cmds, tea.ExecProcess(c, func(err error) tea.Msg {
					return execDoneMsg{err: err}
				}))
			}
		case ext.ActionQuit:
			m.quitting = true
		}
	}

	// Check if a background agent was started
	if m.pendingBgStart != nil {
		ch := m.pendingBgStart.ch
		m.pendingBgStart = nil
		cmds = append(cmds, pollBgEvents(ch))
	}
	return tea.Batch(cmds...)
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

	bgCmd := m.applyActions()

	if m.quitting {
		return m, tea.Quit
	}

	return m, bgCmd
}

func (m Model) handleEvent(evt core.Event) (tea.Model, tea.Cmd) {
	// Dispatch event to registered handlers (event bus)
	if m.cfg.App != nil {
		m.cfg.App.DispatchEvent(context.Background(), evt)
	}

	switch e := evt.(type) {
	case core.EventStreamDelta:
		if e.Kind == "text" {
			m.streamText += e.Delta
			if m.spinnerVerb == "thinking..." {
				m.spinnerVerb = "writing..."
			}
		} else if e.Kind == "thinking" {
			m.streamThink += e.Delta
			if m.spinnerVerb == "thinking..." {
				m.spinnerVerb = "reasoning..."
			}
		}

	case core.EventStreamDone:
		m.streamText = ""
		m.streamThink = ""

	case core.EventToolStart:
		m.activeTool = e.ToolName
		m.spinnerVerb = "running " + e.ToolName + "..."

	case core.EventToolEnd:
		m.activeTool = ""
		m.spinnerVerb = "thinking..."

	case core.EventTurnEnd:
		if e.Assistant != nil {
			m.messages = append(m.messages, e.Assistant)
			m.totalIn += e.Assistant.Usage.InputTokens
			m.totalOut += e.Assistant.Usage.OutputTokens
			m.totalCost += e.Assistant.Usage.Cost
			m.totalCacheRead += e.Assistant.Usage.CacheReadTokens
			m.totalCacheWrite += e.Assistant.Usage.CacheWriteTokens
			m.status.Set(ext.StatusKeyTokens, m.styles.Muted.Render(formatTokens(m.totalIn, m.totalOut, m.totalCacheRead)))
			m.status.Set(ext.StatusKeyCost, m.styles.Muted.Render(formatCost(m.totalCost)))

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
		m.spinnerVerb = ""
		m.status.SetSpinnerView("")
		m.streamCache = StreamCache{}
		m.refreshViewport()
		if m.followOutput {
			m.viewport.GotoBottom()
		}

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

	case core.EventCompact:
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{
				core.TextContent{Text: fmt.Sprintf("Context compacted: %d → %d messages", e.Before, e.After)},
			},
		})
	}

	// Apply actions only after events that can produce them (not stream deltas)
	var actionCmd tea.Cmd
	switch evt.(type) {
	case core.EventAgentEnd, core.EventTurnEnd, core.EventToolEnd, core.EventCompact:
		actionCmd = m.applyActions()
	}

	// Continue polling
	if m.eventCh != nil && m.streaming {
		if actionCmd != nil {
			return m, tea.Batch(pollEvents(m.eventCh), actionCmd)
		}
		return m, pollEvents(m.eventCh)
	}
	return m, actionCmd
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

func (m Model) handleBgEvent(evt core.Event) (tea.Model, tea.Cmd) {
	switch e := evt.(type) {
	case core.EventStreamDelta:
		if e.Kind == "text" {
			m.bgResult.WriteString(e.Delta)
		}

	case core.EventAgentEnd:
		result := strings.TrimSpace(m.bgResult.String())
		if result == "" {
			result = "(background task produced no output)"
		}
		m.messages = append(m.messages, &core.AssistantMessage{
			Content: []core.AssistantContent{
				core.TextContent{Text: fmt.Sprintf("Background task: %s\n\n%s", m.bgTask, result)},
			},
		})
		m.bgAgent = nil
		m.bgEventCh = nil
		m.bgTask = ""
		m.bgResult.Reset()
		m.status.Set(ext.StatusKeyBg, "")
		return m, nil
	}

	// Continue polling
	if m.bgEventCh != nil {
		return m, pollBgEvents(m.bgEventCh)
	}
	return m, nil
}

// pollBgEvents reads the next event from the background agent channel.
func pollBgEvents(ch <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			return bgEventMsg{event: core.EventAgentEnd{}}
		}
		return bgEventMsg{event: evt}
	}
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
func (m *Model) runShortcut(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.cfg.App == nil {
		return m, nil, false
	}

	key := keyString(msg)
	if key == "" {
		return m, nil, false
	}

	shortcuts := m.cfg.App.Shortcuts()
	sc, ok := shortcuts[key]
	if !ok {
		return m, nil, false
	}

	m.bindApp()
	action, _ := sc.Handler(m.cfg.App)
	if action != nil {
		m.cfg.App.EnqueueAction(action)
	}
	cmd := m.applyActions()

	return m, cmd, true
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
