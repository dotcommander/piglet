package ext_test

import (
	"context"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		Handler:     func(a *ext.App) error { return nil },
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

func TestRegisterProvider(t *testing.T) {
	t.Parallel()

	app := ext.NewApp("/tmp")
	app.RegisterProvider("custom", &ext.ProviderConfig{
		BaseURL: "https://custom.ai/v1",
		API:     core.APIOpenAI,
		Models: []core.Model{
			{ID: "custom-1", Name: "Custom Model"},
		},
	})

	providers := app.Providers()
	assert.Contains(t, providers, "custom")
	assert.Equal(t, "https://custom.ai/v1", providers["custom"].BaseURL)
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
