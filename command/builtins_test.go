package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestApp creates an ext.App with all built-in commands registered.
func newTestApp(t *testing.T) *ext.App {
	t.Helper()
	app := ext.NewApp(t.TempDir())
	RegisterBuiltins(app, nil)
	return app
}

// callCmd finds the named command on app and calls its handler with args.
// It returns the first ActionShowMessage text queued, or "" if none.
func callCmd(t *testing.T, app *ext.App, name, args string) error {
	t.Helper()
	cmds := app.Commands()
	cmd, ok := cmds[name]
	require.True(t, ok, "command %q not registered", name)
	return cmd.Handler(args, app)
}

// firstMessage drains pending actions and returns the first ActionShowMessage text.
func firstMessage(t *testing.T, app *ext.App) string {
	t.Helper()
	for _, a := range app.PendingActions() {
		if msg, ok := a.(ext.ActionShowMessage); ok {
			return msg.Text
		}
	}
	return ""
}

// mockAgent implements ext.AgentAPI for tests that need an agent bound.
type mockAgent struct {
	messages []core.Message
	stepMode bool
}

func (m *mockAgent) Steer(_ core.Message)                    {}
func (m *mockAgent) FollowUp(_ core.Message)                 {}
func (m *mockAgent) SetTools(_ []core.Tool)                  {}
func (m *mockAgent) SetModel(_ core.Model)                   {}
func (m *mockAgent) SetProvider(_ core.StreamProvider)       {}
func (m *mockAgent) SetTurnContext(_ []string)               {}
func (m *mockAgent) Messages() []core.Message                { return m.messages }
func (m *mockAgent) IsRunning() bool                         { return false }
func (m *mockAgent) StepMode() bool                          { return m.stepMode }
func (m *mockAgent) SetStepMode(on bool)                     { m.stepMode = on }
func (m *mockAgent) SetMessages(msgs []core.Message)         { m.messages = msgs }
func (m *mockAgent) Provider() core.StreamProvider           { return nil }
func (m *mockAgent) System() string                          { return "" }

// userMsg / assistantMsg build minimal Message implementations for compact tests.
func userMsg(text string) *core.UserMessage {
	return &core.UserMessage{Content: text}
}

func assistantMsg(text string) *core.AssistantMessage {
	return &core.AssistantMessage{
		Content: []core.AssistantContent{core.TextContent{Text: text}},
	}
}

// ---------------------------------------------------------------------------
// registerHelp
// ---------------------------------------------------------------------------

func TestHelpShowsCommandNames(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "help", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	require.NotEmpty(t, msg)
	assert.Contains(t, msg, "Available commands:")
	// A few registered commands should appear in the help output.
	assert.Contains(t, msg, "/help")
	assert.Contains(t, msg, "/compact")
	assert.Contains(t, msg, "/export")
}

func TestHelpShowsShortcuts(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "help", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "ctrl+c")
	assert.Contains(t, msg, keyModel)
	assert.Contains(t, msg, keySession)
}

// ---------------------------------------------------------------------------
// registerClear
// ---------------------------------------------------------------------------

func TestClearIsNoOp(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "clear", "")
	require.NoError(t, err)
	// /clear is intentionally a no-op marker; no actions queued.
	assert.Empty(t, app.PendingActions())
}

// ---------------------------------------------------------------------------
// registerStep
// ---------------------------------------------------------------------------

func TestStepTogglesOn(t *testing.T) {
	t.Parallel()

	agent := &mockAgent{stepMode: false}
	app := newTestApp(t)
	app.Bind(agent)

	err := callCmd(t, app, "step", "")
	require.NoError(t, err)

	assert.True(t, agent.stepMode)
	msg := firstMessage(t, app)
	assert.Contains(t, msg, "on")
}

func TestStepTogglesOff(t *testing.T) {
	t.Parallel()

	agent := &mockAgent{stepMode: true}
	app := newTestApp(t)
	app.Bind(agent)

	err := callCmd(t, app, "step", "")
	require.NoError(t, err)

	assert.False(t, agent.stepMode)
	msg := firstMessage(t, app)
	assert.Contains(t, msg, "off")
}

func TestStepWithoutAgentBound(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	// No agent bound — ToggleStepMode returns false, message says "off".
	err := callCmd(t, app, "step", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Step mode:")
}

// ---------------------------------------------------------------------------
// registerCompact
// ---------------------------------------------------------------------------

func TestCompactTooFewMessages(t *testing.T) {
	t.Parallel()

	agent := &mockAgent{messages: []core.Message{
		userMsg("hello"),
		assistantMsg("hi"),
	}}
	app := newTestApp(t)
	app.Bind(agent)

	err := callCmd(t, app, "compact", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Not enough messages")
}

func TestCompactStaticFallback(t *testing.T) {
	t.Parallel()

	// Build a conversation with enough messages (>=4).
	msgs := []core.Message{
		userMsg("msg1"), assistantMsg("reply1"),
		userMsg("msg2"), assistantMsg("reply2"),
		userMsg("msg3"), assistantMsg("reply3"),
		userMsg("msg4"), assistantMsg("reply4"),
		userMsg("msg5"), assistantMsg("reply5"),
	}
	agent := &mockAgent{messages: msgs}
	app := newTestApp(t)
	app.Bind(agent)

	before := len(agent.messages)
	err := callCmd(t, app, "compact", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Compacted:")

	// The static compactor keeps last 7 messages plus a summary message.
	assert.Less(t, len(agent.messages), before)
}

// ---------------------------------------------------------------------------
// registerExport
// ---------------------------------------------------------------------------

func TestExportNoMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	app.Bind(&mockAgent{})

	err := callCmd(t, app, "export", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "No messages to export")
}

func TestExportWritesFile(t *testing.T) {
	t.Parallel()

	agent := &mockAgent{messages: []core.Message{
		userMsg("hello"),
		assistantMsg("world"),
	}}
	app := newTestApp(t)
	app.Bind(agent)

	// Capture working dir to verify file was created there.
	wd, err := os.Getwd()
	require.NoError(t, err)

	exportErr := callCmd(t, app, "export", "")
	require.NoError(t, exportErr)

	msg := firstMessage(t, app)
	require.Contains(t, msg, "Exported to ")

	// Extract the filename from the message.
	filename := strings.TrimPrefix(msg, "Exported to ")
	filename = strings.TrimSpace(filename)

	path := filepath.Join(wd, filename)
	t.Cleanup(func() { _ = os.Remove(path) })

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)

	content := string(data)
	assert.Contains(t, content, "# Piglet Conversation")
	assert.Contains(t, content, "## User")
	assert.Contains(t, content, "hello")
	assert.Contains(t, content, "## Assistant")
	assert.Contains(t, content, "world")
}

// ---------------------------------------------------------------------------
// registerTitle
// ---------------------------------------------------------------------------

func TestTitleRequiresArg(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "title", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Usage:")
}

func TestTitleWithoutSessionManager(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	// No session manager bound — SetSessionTitle will return error.
	err := callCmd(t, app, "title", "My Title")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Failed to set title")
}

func TestTitleWithSessionManager(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	app.Bind(nil, ext.WithSessionManager(&mockSessionMgr{}))

	err := callCmd(t, app, "title", "  My Session  ")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "My Session")
}

// ---------------------------------------------------------------------------
// registerBg
// ---------------------------------------------------------------------------

func TestBgRequiresPrompt(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "bg", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Usage:")
}

func TestBgWithoutCallback(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	// No background callback bound — RunBackground returns error.
	err := callCmd(t, app, "bg", "do something")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Background task failed")
}

func TestBgStarted(t *testing.T) {
	t.Parallel()

	var capturedPrompt string
	app := newTestApp(t)
	app.Bind(nil, ext.WithRunBackground(func(p string) error {
		capturedPrompt = p
		return nil
	}))

	err := callCmd(t, app, "bg", "analyze logs")
	require.NoError(t, err)

	assert.Equal(t, "analyze logs", capturedPrompt)
	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Background task started")
}

// ---------------------------------------------------------------------------
// registerBgCancel
// ---------------------------------------------------------------------------

func TestBgCancelWhenNotRunning(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "bg-cancel", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "No background task running")
}

func TestBgCancelWhenRunning(t *testing.T) {
	t.Parallel()

	cancelled := false
	app := newTestApp(t)
	app.Bind(nil,
		ext.WithIsBackgroundRunning(func() bool { return true }),
		ext.WithCancelBackground(func() { cancelled = true }),
	)

	err := callCmd(t, app, "bg-cancel", "")
	require.NoError(t, err)

	assert.True(t, cancelled)
	msg := firstMessage(t, app)
	assert.Contains(t, msg, "cancelled")
}

// ---------------------------------------------------------------------------
// registerSearch
// ---------------------------------------------------------------------------

func TestSearchRequiresQuery(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "search", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "Usage:")
}

func TestSearchNoSessionManager(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	// No session manager — Sessions() returns an error.
	err := callCmd(t, app, "search", "myproject")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "sessions not configured")
}

func TestSearchNoResults(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	app.Bind(nil, ext.WithSessionManager(&mockSessionMgr{
		sessions: []ext.SessionSummary{
			{ID: "aabbcc", Title: "completely different", Path: "/tmp/s1.jsonl"},
		},
	}))

	err := callCmd(t, app, "search", "xyz-no-match")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "No sessions matching")
}

func TestSearchFindsMatchByTitle(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	app.Bind(nil, ext.WithSessionManager(&mockSessionMgr{
		sessions: []ext.SessionSummary{
			{ID: "aabbcc", Title: "refactor auth", Path: "/tmp/s1.jsonl"},
			{ID: "ddeeff", Title: "update readme", Path: "/tmp/s2.jsonl"},
		},
	}))

	err := callCmd(t, app, "search", "auth")
	require.NoError(t, err)

	// Should show a picker with 1 result.
	for _, a := range app.PendingActions() {
		if picker, ok := a.(ext.ActionShowPicker); ok {
			assert.Contains(t, picker.Title, "auth")
			assert.Len(t, picker.Items, 1)
			return
		}
	}
	t.Fatal("expected ActionShowPicker to be enqueued")
}

// ---------------------------------------------------------------------------
// registerQuit
// ---------------------------------------------------------------------------

func TestQuitEnqueuesQuitAction(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "quit", "")
	require.NoError(t, err)

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	_, ok := actions[0].(ext.ActionQuit)
	assert.True(t, ok)
}

// ---------------------------------------------------------------------------
// sessionPickerItems (pure function)
// ---------------------------------------------------------------------------

func TestSessionPickerItemsFallbackTitle(t *testing.T) {
	t.Parallel()

	summaries := []ext.SessionSummary{
		{
			ID:        "abcdefgh1234",
			Title:     "",
			Path:      "/tmp/s.jsonl",
			CreatedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		},
	}
	items := sessionPickerItems(summaries)
	require.Len(t, items, 1)
	// Falls back to first 8 chars of ID.
	assert.Equal(t, "abcdefgh", items[0].Label)
}

func TestSessionPickerItemsWithTitle(t *testing.T) {
	t.Parallel()

	summaries := []ext.SessionSummary{
		{
			ID:        "abcdefgh1234",
			Title:     "My Session",
			Path:      "/tmp/s.jsonl",
			CreatedAt: time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC),
		},
	}
	items := sessionPickerItems(summaries)
	require.Len(t, items, 1)
	assert.Equal(t, "My Session", items[0].Label)
	assert.Equal(t, "/tmp/s.jsonl", items[0].ID)
}

func TestSessionPickerItemsWithCWD(t *testing.T) {
	t.Parallel()

	summaries := []ext.SessionSummary{
		{
			ID:        "aabbcc001122",
			Title:     "Work",
			CWD:       "/home/gary/project",
			Path:      "/tmp/s.jsonl",
			CreatedAt: time.Now(),
		},
	}
	items := sessionPickerItems(summaries)
	require.Len(t, items, 1)
	assert.Contains(t, items[0].Desc, "/home/gary/project")
}

func TestSessionPickerItemsWithParent(t *testing.T) {
	t.Parallel()

	summaries := []ext.SessionSummary{
		{
			ID:        "child123",
			Title:     "Child",
			ParentID:  "parentXYZABC",
			Path:      "/tmp/s.jsonl",
			CreatedAt: time.Now(),
		},
	}
	items := sessionPickerItems(summaries)
	require.Len(t, items, 1)
	assert.Contains(t, items[0].Desc, "forked from")
	assert.Contains(t, items[0].Desc, "parentXY") // first 8 chars
}

func TestSessionPickerItemsEmpty(t *testing.T) {
	t.Parallel()

	items := sessionPickerItems(nil)
	assert.Empty(t, items)
}

// ---------------------------------------------------------------------------
// exportMarkdown (pure function)
// ---------------------------------------------------------------------------

func TestExportMarkdownUserAndAssistant(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		userMsg("hello there"),
		assistantMsg("hello back"),
	}

	path := filepath.Join(t.TempDir(), "export.md")
	err := exportMarkdown(msgs, path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "# Piglet Conversation")
	assert.Contains(t, content, "## User")
	assert.Contains(t, content, "hello there")
	assert.Contains(t, content, "## Assistant")
	assert.Contains(t, content, "hello back")
}

func TestExportMarkdownToolResult(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.ToolResultMessage{
			ToolName: "bash",
			Content:  []core.ContentBlock{core.TextContent{Text: "$ ls\nfoo.go"}},
		},
	}

	path := filepath.Join(t.TempDir(), "export.md")
	err := exportMarkdown(msgs, path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "### Tool: bash")
	assert.Contains(t, content, "foo.go")
}

func TestExportMarkdownThinkingContent(t *testing.T) {
	t.Parallel()

	msgs := []core.Message{
		&core.AssistantMessage{
			Content: []core.AssistantContent{
				core.ThinkingContent{Thinking: "let me reason..."},
				core.TextContent{Text: "final answer"},
			},
		},
	}

	path := filepath.Join(t.TempDir(), "export.md")
	err := exportMarkdown(msgs, path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "<details><summary>Thinking</summary>")
	assert.Contains(t, content, "let me reason...")
	assert.Contains(t, content, "final answer")
}

func TestExportMarkdownEmpty(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "export.md")
	err := exportMarkdown(nil, path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# Piglet Conversation")
}

// ---------------------------------------------------------------------------
// RegisterBuiltins — registration completeness
// ---------------------------------------------------------------------------

func TestRegisterBuiltinsRegistersExpectedCommands(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	RegisterBuiltins(app, nil)

	cmds := app.Commands()
	expected := []string{
		"help", "clear", "step", "compact", "export",
		"extensions", "ext-init", "model", "session",
		"models-sync", "branch", "bg", "bg-cancel",
		"search", "title", "undo", "config", "quit",
	}
	for _, name := range expected {
		assert.Contains(t, cmds, name, "expected command %q to be registered", name)
	}
}

func TestRegisterBuiltinsRegistersShortcuts(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	RegisterBuiltins(app, nil)

	shortcuts := app.Shortcuts()
	assert.Contains(t, shortcuts, keyModel)
	assert.Contains(t, shortcuts, keySession)
}

func TestRegisterBuiltinsCustomShortcutOverride(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	RegisterBuiltins(app, map[string]string{
		shortcutModel: "ctrl+m",
	})

	shortcuts := app.Shortcuts()
	assert.Contains(t, shortcuts, "ctrl+m", "custom key should override default")
}

// ---------------------------------------------------------------------------
// mockSessionMgr (package-level, reusable across tests)
// ---------------------------------------------------------------------------

type mockSessionMgr struct {
	sessions  []ext.SessionSummary
	loadErr   error
	titleErr  error
}

func (m *mockSessionMgr) List() ([]ext.SessionSummary, error) { return m.sessions, nil }
func (m *mockSessionMgr) Load(_ string) (any, error)          { return nil, m.loadErr }
func (m *mockSessionMgr) Fork() (string, any, int, error)     { return "", nil, 0, nil }
func (m *mockSessionMgr) SetTitle(_ string) error             { return m.titleErr }
func (m *mockSessionMgr) Title() string                       { return "" }
