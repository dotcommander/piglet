package command

import (
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
	RegisterBuiltins(app, nil, "test")
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

func (m *mockAgent) Steer(_ core.Message)              {}
func (m *mockAgent) FollowUp(_ core.Message)           {}
func (m *mockAgent) SetModel(_ core.Model)             {}
func (m *mockAgent) SetProvider(_ core.StreamProvider) {}
func (m *mockAgent) Messages() []core.Message          { return m.messages }
func (m *mockAgent) StepMode() bool                    { return m.stepMode }
func (m *mockAgent) SetStepMode(on bool)               { m.stepMode = on }
func (m *mockAgent) SetMessages(msgs []core.Message)   { m.messages = msgs }
func (m *mockAgent) Provider() core.StreamProvider     { return nil }
func (m *mockAgent) System() string                    { return "" }

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
}

func TestHelpShowsModelShortcut(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	err := callCmd(t, app, "help", "")
	require.NoError(t, err)

	msg := firstMessage(t, app)
	assert.Contains(t, msg, "ctrl+c")
	assert.Contains(t, msg, keyModel)
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
// RegisterBuiltins — registration completeness
// ---------------------------------------------------------------------------

func TestRegisterBuiltinsRegistersExpectedCommands(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	RegisterBuiltins(app, nil, "test")

	cmds := app.Commands()
	// Session commands (session, fork, branch, search, tree, title) have moved to
	// extensions/sessioncmd and are no longer compiled-in. Only the model command
	// and core lifecycle commands remain.
	expected := []string{
		"help", "clear", "step", "compact",
		"model", "quit",
	}
	for _, name := range expected {
		assert.Contains(t, cmds, name, "expected command %q to be registered", name)
	}
}

func TestRegisterBuiltinsRegistersModelShortcut(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	RegisterBuiltins(app, nil, "test")

	shortcuts := app.Shortcuts()
	assert.Contains(t, shortcuts, keyModel)
	// ctrl+s (session picker) has moved to sessioncmd extension — not registered here.
	assert.NotContains(t, shortcuts, "ctrl+s")
}

func TestRegisterBuiltinsCustomShortcutOverride(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	RegisterBuiltins(app, map[string]string{
		shortcutModel: "ctrl+m",
	}, "test")

	shortcuts := app.Shortcuts()
	assert.Contains(t, shortcuts, "ctrl+m", "custom key should override default")
}

// Keep the time import used by legacy test helpers.
var _ = time.Now
