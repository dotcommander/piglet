package ext_test

import (
	"context"
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSessionMgr implements ext.SessionManager for testing.
type mockSessionMgr struct {
	sessions   []ext.SessionSummary
	loadResult any
	loadErr    error
	forkParent string
	forkResult any
	forkCount  int
	forkErr    error
	titleErr   error
}

func (m *mockSessionMgr) List() ([]ext.SessionSummary, error) {
	return m.sessions, nil
}

func (m *mockSessionMgr) Load(path string) (any, error) {
	return m.loadResult, m.loadErr
}

func (m *mockSessionMgr) Fork() (string, any, int, error) {
	return m.forkParent, m.forkResult, m.forkCount, m.forkErr
}

func (m *mockSessionMgr) Branch(entryID string) (any, error) {
	return nil, nil
}

func (m *mockSessionMgr) BranchWithSummary(entryID, summary string) (any, error) {
	return nil, nil
}

func (m *mockSessionMgr) EntryInfos() []ext.EntryInfo {
	return nil
}

func (m *mockSessionMgr) SetTitle(title string) error {
	return m.titleErr
}

func (m *mockSessionMgr) Title() string {
	return ""
}

func (m *mockSessionMgr) AppendEntry(kind string, data any) error {
	return nil
}

func (m *mockSessionMgr) AppendCustomMessage(role, content string) error {
	return nil
}

func (m *mockSessionMgr) AppendLabel(targetID, label string) error {
	return nil
}

func (m *mockSessionMgr) FullTree() []ext.TreeNode {
	return nil
}

func (m *mockSessionMgr) ResetLeaf() (any, error) {
	return nil, nil
}

// mockModelMgr implements ext.ModelManager for testing.
type mockModelMgr struct {
	models    []core.Model
	switchMod core.Model
	switchErr error
	syncCount int
	syncErr   error
}

func (m *mockModelMgr) Available() []core.Model {
	return m.models
}

func (m *mockModelMgr) Switch(id string) (core.Model, core.StreamProvider, error) {
	return m.switchMod, nil, m.switchErr
}

func (m *mockModelMgr) Sync() (int, error) {
	return m.syncCount, m.syncErr
}

func (m *mockModelMgr) WriteWithOverrides(_ map[string]ext.ModelOverride) (int, error) {
	return m.syncCount, m.syncErr
}

func echoToolDef() *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "echo",
			Description: "Echoes input",
			Parameters:  map[string]any{"type": "object"},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			text, _ := args["text"].(string)
			return &core.ToolResult{
				Content: []core.ContentBlock{core.TextContent{Text: text}},
			}, nil
		},
	}
}

func TestRegisterTool(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterTool(echoToolDef())

	tools := app.Tools()
	assert.Equal(t, []string{"echo"}, tools)
}

func TestCoreToolsConversion(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterTool(echoToolDef())

	coreTools := app.CoreTools()
	require.Len(t, coreTools, 1)
	assert.Equal(t, "echo", coreTools[0].Name)

	// Execute should work
	result, err := coreTools[0].Execute(context.Background(), "tc1", map[string]any{"text": "hello"})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "hello", result.Content[0].(core.TextContent).Text)
}

func TestRegisterCommand(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterCommand(&ext.Command{
		Name:        "greet",
		Description: "Say hello",
		Handler: func(args string, a *ext.App) error {
			return nil
		},
	})

	cmds := app.Commands()
	assert.Contains(t, cmds, "greet")
	assert.Equal(t, "Say hello", cmds["greet"].Description)
}

func TestRegisterShortcut(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterShortcut(&ext.Shortcut{
		Key:         "ctrl+g",
		Description: "Git status",
		Handler:     func(a *ext.App) (ext.Action, error) { return nil, nil },
	})

	shortcuts := app.Shortcuts()
	assert.Contains(t, shortcuts, "ctrl+g")
}

func TestInterceptorBeforeModifiesArgs(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterInterceptor(ext.Interceptor{
		Name:     "modifier",
		Priority: 1000,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			modified := make(map[string]any)
			for k, v := range args {
				modified[k] = v
			}
			modified["injected"] = true
			return true, modified, nil
		},
	})
	app.RegisterTool(echoToolDef())

	coreTools := app.CoreTools()
	require.Len(t, coreTools, 1)

	// The interceptor should have injected "injected: true"
	result, err := coreTools[0].Execute(context.Background(), "tc1", map[string]any{"text": "hi"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestInterceptorBeforeBlocks(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterInterceptor(ext.Interceptor{
		Name:     "blocker",
		Priority: 2000,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			return false, nil, nil
		},
	})
	app.RegisterTool(echoToolDef())

	coreTools := app.CoreTools()
	result, err := coreTools[0].Execute(context.Background(), "tc1", map[string]any{"text": "blocked"})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Contains(t, result.Content[0].(core.TextContent).Text, "blocked by interceptor")
}

func TestInterceptorPriorityOrder(t *testing.T) {
	t.Parallel()

	var order []string

	app := ext.NewApp("/tmp")
	app.RegisterInterceptor(ext.Interceptor{
		Name:     "low",
		Priority: 100,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			order = append(order, "low")
			return true, args, nil
		},
	})
	app.RegisterInterceptor(ext.Interceptor{
		Name:     "high",
		Priority: 2000,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			order = append(order, "high")
			return true, args, nil
		},
	})
	app.RegisterTool(echoToolDef())

	coreTools := app.CoreTools()
	_, err := coreTools[0].Execute(context.Background(), "tc1", map[string]any{"text": "test"})
	require.NoError(t, err)

	// High priority should run first
	require.Len(t, order, 2)
	assert.Equal(t, "high", order[0])
	assert.Equal(t, "low", order[1])
}

func TestRegisterRenderer(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterRenderer("diff", func(message any, expanded bool) string {
		return "rendered diff"
	})

	renderers := app.Renderers()
	assert.Contains(t, renderers, "diff")
	assert.Equal(t, "rendered diff", renderers["diff"](nil, true))
}

func TestExtensionFunction(t *testing.T) {
	t.Parallel()

	// Simulate a real extension registration
	myExtension := ext.Extension(func(app *ext.App) error {
		app.RegisterTool(&ext.ToolDef{
			ToolSchema: core.ToolSchema{
				Name:        "my_tool",
				Description: "Does things",
			},
			Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
				return &core.ToolResult{
					Content: []core.ContentBlock{core.TextContent{Text: "done"}},
				}, nil
			},
		})
		app.RegisterCommand(&ext.Command{
			Name:        "my_cmd",
			Description: "My command",
			Handler:     func(args string, a *ext.App) error { return nil },
		})
		return nil
	})

	app := ext.NewApp("/tmp")
	err := myExtension(app)
	require.NoError(t, err)

	assert.Equal(t, []string{"my_tool"}, app.Tools())
	assert.Contains(t, app.Commands(), "my_cmd")
}

func TestCWD(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/home/user/project")
	assert.Equal(t, "/home/user/project", app.CWD())
}

// ---------------------------------------------------------------------------
// Action queue tests
// ---------------------------------------------------------------------------

func TestShowMessageEnqueuesAction(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.ShowMessage("hello")

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act, ok := actions[0].(ext.ActionShowMessage)
	require.True(t, ok)
	assert.Equal(t, "hello", act.Text)
}

func TestNotifyEnqueuesAction(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.Notify("alert")

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act, ok := actions[0].(ext.ActionNotify)
	require.True(t, ok)
	assert.Equal(t, "alert", act.Message)
}

func TestRequestQuitEnqueuesAction(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.RequestQuit()

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	_, ok := actions[0].(ext.ActionQuit)
	assert.True(t, ok)
}

func TestSetStatusEnqueuesAction(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.SetStatus("model", "claude-sonnet")

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act, ok := actions[0].(ext.ActionSetStatus)
	require.True(t, ok)
	assert.Equal(t, "model", act.Key)
	assert.Equal(t, "claude-sonnet", act.Text)
}

func TestShowPickerEnqueuesAction(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")

	called := false
	items := []ext.PickerItem{{ID: "1", Label: "One"}}
	app.ShowPicker("Pick", items, func(p ext.PickerItem) { called = true })

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act, ok := actions[0].(ext.ActionShowPicker)
	require.True(t, ok)
	assert.Equal(t, "Pick", act.Title)
	require.Len(t, act.Items, 1)
	assert.Equal(t, "One", act.Items[0].Label)

	// Callback should be preserved
	act.OnSelect(ext.PickerItem{})
	assert.True(t, called)
}

func TestLoadSessionEnqueuesSwapAction(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.Bind(nil, ext.WithSessionManager(&mockSessionMgr{
		loadResult: "fake-session-obj",
	}))

	err := app.LoadSession("/path/to/session.jsonl")
	require.NoError(t, err)

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act, ok := actions[0].(ext.ActionSwapSession)
	require.True(t, ok)
	assert.Equal(t, "fake-session-obj", act.Session)
}

func TestPendingActionsClearsQueue(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.ShowMessage("first")
	app.ShowMessage("second")

	actions := app.PendingActions()
	assert.Len(t, actions, 2)

	// Second drain should be empty
	actions = app.PendingActions()
	assert.Empty(t, actions)
}

func TestMultipleActionsPreserveOrder(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.ShowMessage("msg")
	app.SetStatus("model", "claude")
	app.RequestQuit()

	actions := app.PendingActions()
	require.Len(t, actions, 3)
	assert.IsType(t, ext.ActionShowMessage{}, actions[0])
	assert.IsType(t, ext.ActionSetStatus{}, actions[1])
	assert.IsType(t, ext.ActionQuit{}, actions[2])
}

func TestBindClearsStaleActions(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.ShowMessage("stale")

	// Bind should clear previous actions
	app.Bind(nil)

	actions := app.PendingActions()
	assert.Empty(t, actions)
}

func TestCommandHandlerActionsTestable(t *testing.T) {
	t.Parallel()

	// Simulate testing a command handler without a TUI
	app := ext.NewApp("/tmp")
	app.RegisterCommand(&ext.Command{
		Name:        "greet",
		Description: "Greet the user",
		Handler: func(args string, a *ext.App) error {
			a.ShowMessage("Hello, " + args + "!")
			return nil
		},
	})

	cmds := app.Commands()
	err := cmds["greet"].Handler("world", app)
	require.NoError(t, err)

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act := actions[0].(ext.ActionShowMessage)
	assert.Equal(t, "Hello, world!", act.Text)
}

func TestMultipleTools(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	for _, name := range []string{"read", "write", "edit", "bash", "grep", "find", "ls"} {
		app.RegisterTool(&ext.ToolDef{
			ToolSchema: core.ToolSchema{Name: name, Description: name + " tool"},
			Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
				return &core.ToolResult{Content: []core.ContentBlock{core.TextContent{Text: "ok"}}}, nil
			},
		})
	}

	tools := app.Tools()
	assert.Len(t, tools, 7)
	assert.Equal(t, "bash", tools[0]) // sorted
}

// ---------------------------------------------------------------------------
// Domain facade tests (SessionManager, ModelManager)
// ---------------------------------------------------------------------------

func TestSessionsWithoutManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")

	_, err := app.Sessions()
	assert.ErrorContains(t, err, "no active session")
}

func TestSessionsWithManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.Bind(nil, ext.WithSessionManager(&mockSessionMgr{
		sessions: []ext.SessionSummary{
			{ID: "abc", Title: "Test"},
		},
	}))

	summaries, err := app.Sessions()
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Test", summaries[0].Title)
}

func TestForkSessionEnqueuesSwap(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.Bind(nil, ext.WithSessionManager(&mockSessionMgr{
		forkParent: "abcd1234",
		forkResult: "forked-session",
		forkCount:  5,
	}))

	parentID, count, err := app.ForkSession()
	require.NoError(t, err)
	assert.Equal(t, "abcd1234", parentID)
	assert.Equal(t, 5, count)

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act, ok := actions[0].(ext.ActionSwapSession)
	require.True(t, ok)
	assert.Equal(t, "forked-session", act.Session)
}

func TestSetSessionTitleWithManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.Bind(nil, ext.WithSessionManager(&mockSessionMgr{}))

	err := app.SetSessionTitle("new title")
	assert.NoError(t, err)
}

func TestSetSessionTitleWithoutManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")

	err := app.SetSessionTitle("title")
	assert.ErrorContains(t, err, "no active session")
}

func TestAvailableModelsWithManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.Bind(nil, ext.WithModelManager(&mockModelMgr{
		models: []core.Model{{ID: "test", Name: "Test Model"}},
	}))

	models := app.AvailableModels()
	require.Len(t, models, 1)
	assert.Equal(t, "Test Model", models[0].Name)
}

func TestAvailableModelsWithoutManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")

	models := app.AvailableModels()
	assert.Empty(t, models)
}

func TestSwitchModelEnqueuesStatus(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.Bind(nil, ext.WithModelManager(&mockModelMgr{
		switchMod: core.Model{ID: "claude-3", Name: "Claude 3", Provider: "anthropic"},
	}))

	err := app.SwitchModel("anthropic/claude-3")
	require.NoError(t, err)

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act, ok := actions[0].(ext.ActionSetStatus)
	require.True(t, ok)
	assert.Equal(t, "model", act.Key)
}

func TestSwitchModelWithoutManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")

	err := app.SwitchModel("test/model")
	assert.ErrorContains(t, err, "model manager not configured")
}

func TestSyncModelsWithManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")
	app.Bind(nil, ext.WithModelManager(&mockModelMgr{syncCount: 3}))

	updated, err := app.SyncModels()
	require.NoError(t, err)
	assert.Equal(t, 3, updated)
}

func TestSyncModelsWithoutManager(t *testing.T) {
	t.Parallel()
	app := ext.NewApp("/tmp")

	_, err := app.SyncModels()
	assert.ErrorContains(t, err, "model manager not configured")
}

// ---------------------------------------------------------------------------
// Interceptor call-time evaluation
// ---------------------------------------------------------------------------

func TestInterceptorRegisteredAfterToolStillFires(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")

	// Register tool FIRST
	app.RegisterTool(echoToolDef())

	// Register interceptor AFTER the tool
	fired := false
	app.RegisterInterceptor(ext.Interceptor{
		Name:     "late-registerer",
		Priority: 1000,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			fired = true
			return true, args, nil
		},
	})

	// Get CoreTools after both are registered — interceptor must still fire
	coreTools := app.CoreTools()
	require.Len(t, coreTools, 1)

	_, err := coreTools[0].Execute(context.Background(), "tc1", map[string]any{"text": "test"})
	require.NoError(t, err)
	assert.True(t, fired, "interceptor registered after tool should still fire at call time")
}

func TestInterceptorRegisteredAfterCoreToolsStillFires(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterTool(echoToolDef())

	// Get CoreTools BEFORE registering the interceptor
	coreTools := app.CoreTools()

	// Register interceptor AFTER CoreTools() was called
	fired := false
	app.RegisterInterceptor(ext.Interceptor{
		Name:     "very-late",
		Priority: 1000,
		Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			fired = true
			return true, args, nil
		},
	})

	// The interceptor should still fire because evaluation happens at call time
	_, err := coreTools[0].Execute(context.Background(), "tc1", map[string]any{"text": "test"})
	require.NoError(t, err)
	assert.True(t, fired, "interceptor registered after CoreTools() should fire at call time")
}
