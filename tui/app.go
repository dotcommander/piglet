package tui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

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
	Agent   *core.Agent
	Session *session.Session
	Model   core.Model
	Models  []core.Model // available models from registry
	SessDir string       // session directory path
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

// titleGeneratedMsg carries an auto-generated session title.
type titleGeneratedMsg struct{ title string }

// notifyTickMsg decrements the notification timer.
type notifyTickMsg struct{}

// clipboardImageMsg carries the result of a clipboard image read.
type clipboardImageMsg struct {
	image *core.ImageContent
	err   error
}

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

	// State
	messages       []core.Message
	streaming      bool
	streamText     string
	streamThink    string
	activeTool     string
	totalIn        int
	totalOut       int
	totalCost      float64
	quitting       bool
	pickerCallback func(ext.PickerItem)

	// Background start channel (set by bind callback, read after command)
	pendingBgStart *bgStartResult

	// Event channel
	eventCh <-chan core.Event

	// Background agent state
	bgAgent   *core.Agent
	bgEventCh <-chan core.Event
	bgTask    string
	bgResult  *strings.Builder

	// Auto-title: generate after first exchange
	titleGenerated bool

	// Streaming glamour cache
	streamCache StreamCache

	// Toast notification (transient, not in conversation history)
	notification      string
	notificationTimer int // ticks remaining (0 = hidden)

	// Pending image attachment for next message
	pendingImage *core.ImageContent
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

	return Model{
		cfg:      cfg,
		styles:   styles,
		input:    NewInputModel(styles, commands),
		status:   status,
		msgView:  NewMessageView(styles, 80),
		bgResult: &strings.Builder{},
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
				m.streamCache = StreamCache{}
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case msg.Code == 'i' && msg.Mod.Contains(tea.ModCtrl):
			return m.handleImageAttach()

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

	case titleGeneratedMsg:
		if m.cfg.Session != nil && msg.title != "" {
			_ = m.cfg.Session.SetTitle(msg.title)
		}
		return m, nil

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

	case clipboardImageMsg:
		if msg.err != nil {
			m.showNotification("No image: " + msg.err.Error())
			return m, notifyTick()
		}
		m.pendingImage = msg.image
		m.input.SetAttachment("image")
		size := len(msg.image.Data) * 3 / 4 // approximate decoded size
		m.showNotification(fmt.Sprintf("Image attached (%s) — send with your next message", formatImageSize(size)))
		return m, notifyTick()
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

	// Messages viewport — bottom-aligned with top padding for breathing room
	content := "\n" + m.renderMessages()
	contentLines := strings.Count(content, "\n")
	vpHeight := m.viewport.Height()
	if contentLines < vpHeight {
		content = strings.Repeat("\n", vpHeight-contentLines) + content
	}
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
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

// handleImageAttach reads an image from the macOS clipboard and attaches it to the next message.
func (m Model) handleImageAttach() (tea.Model, tea.Cmd) {
	if m.pendingImage != nil {
		// Toggle off
		m.pendingImage = nil
		m.input.SetAttachment("")
		m.showNotification("Image attachment removed")
		return m, notifyTick()
	}

	return m, func() tea.Msg {
		return readClipboardImage()
	}
}

// readClipboardImage reads a PNG image from the macOS clipboard.
func readClipboardImage() clipboardImageMsg {
	cmd := exec.Command("osascript", "-e", "the clipboard info")
	info, err := cmd.Output()
	if err != nil {
		return clipboardImageMsg{err: fmt.Errorf("clipboard not available")}
	}

	// Check if clipboard has image data
	infoStr := string(info)
	var mime string
	switch {
	case strings.Contains(infoStr, "PNGf"):
		mime = "image/png"
	case strings.Contains(infoStr, "JPEG"):
		mime = "image/jpeg"
	default:
		return clipboardImageMsg{err: fmt.Errorf("no image in clipboard")}
	}

	// Read the image data
	pbCmd := exec.Command("osascript", "-e",
		`set imageData to the clipboard as «class PNGf»
set theFile to (open for access POSIX file "/dev/stdout" with write permission)
write imageData to theFile
close access theFile`)
	data, err := pbCmd.Output()
	if err != nil || len(data) == 0 {
		return clipboardImageMsg{err: fmt.Errorf("failed to read image from clipboard")}
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return clipboardImageMsg{
		image: &core.ImageContent{
			Data:     encoded,
			MimeType: mime,
		},
	}
}

func formatTokens(in, out int) string {
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

	// Start agent
	ch := m.cfg.Agent.Start(context.Background(), text)
	m.eventCh = ch
	m.streaming = true

	return m, tea.Batch(pollEvents(ch), tickCmd())
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
			m.totalCost += e.Assistant.Usage.Cost
			m.status.Set(ext.StatusKeyTokens, m.styles.Muted.Render(formatTokens(m.totalIn, m.totalOut)))
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
		m.streamCache = StreamCache{}

		// Auto-generate session title after first exchange
		autoTitle := m.cfg.Settings == nil || m.cfg.Settings.Agent.AutoTitleEnabled()
		if autoTitle && !m.titleGenerated && m.cfg.Session != nil && m.cfg.Session.Meta().Title == "" {
			m.titleGenerated = true
			if cmd := m.generateTitle(); cmd != nil {
				return m, cmd
			}
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

// generateTitle fires a lightweight LLM call to produce a short session title.
// Returns a tea.Cmd that runs in a goroutine and sends titleGeneratedMsg.
func (m *Model) generateTitle() tea.Cmd {
	prov := m.cfg.Agent.Provider()
	if prov == nil {
		return nil
	}

	// Collect the first user message and first assistant text
	var userText, assistantText string
	for _, msg := range m.messages {
		switch v := msg.(type) {
		case *core.UserMessage:
			if userText == "" {
				userText = v.Content
			}
		case *core.AssistantMessage:
			if assistantText == "" {
				for _, c := range v.Content {
					if tc, ok := c.(core.TextContent); ok {
						assistantText = tc.Text
						break
					}
				}
			}
		}
		if userText != "" && assistantText != "" {
			break
		}
	}

	if userText == "" {
		return nil
	}

	// Truncate to keep the title prompt small
	if len([]rune(userText)) > 200 {
		userText = string([]rune(userText)[:200])
	}
	if len([]rune(assistantText)) > 200 {
		assistantText = string([]rune(assistantText)[:200])
	}

	maxTok := 30
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		ch := prov.Stream(ctx, core.StreamRequest{
			System: "Generate a concise 3-5 word title for this conversation. Reply with ONLY the title, no quotes or punctuation.",
			Messages: []core.Message{
				&core.UserMessage{Content: fmt.Sprintf("User: %s\n\nAssistant: %s", userText, assistantText)},
			},
			Options: core.StreamOptions{MaxTokens: &maxTok},
		})

		var title strings.Builder
		for evt := range ch {
			if evt.Type == core.StreamTextDelta {
				title.WriteString(evt.Delta)
			}
		}

		result := strings.TrimSpace(title.String())
		if result == "" {
			return titleGeneratedMsg{}
		}
		// Cap at 50 runes
		if runes := []rune(result); len(runes) > 50 {
			result = string(runes[:50])
		}
		return titleGeneratedMsg{title: result}
	}
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
	_ = m.applyActions()

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
