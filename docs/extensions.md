# Extensions

Extensions add custom functionality to piglet: tools, slash commands, keyboard shortcuts, prompt sections, and interceptors.

All of piglet's built-in functionality (7 tools, 8 commands, 2 shortcuts) registers through the same extension API, so anything built-in can be overridden or extended.

## Writing an Extension

An extension is a Go function that receives `*ext.App` and registers capabilities:

```go
func Register(app *ext.App) error {
    // Register tools, commands, shortcuts, prompt sections, interceptors
    return nil
}
```

## Tools

Tools are functions the LLM can call. Each tool needs a name, description, JSON Schema parameters, and an execute function.

```go
app.RegisterTool(&ext.ToolDef{
    ToolSchema: core.ToolSchema{
        Name:        "weather",
        Description: "Get current weather for a city",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "city": map[string]any{
                    "type":        "string",
                    "description": "City name",
                },
            },
            "required": []string{"city"},
        },
    },
    PromptHint: "Check weather conditions",
    Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
        city := args["city"].(string)
        // ... fetch weather ...
        return &core.ToolResult{
            Content: []core.ContentBlock{
                core.TextContent{Text: "72F, sunny in " + city},
            },
        }, nil
    },
})
```

## Slash Commands

Commands are invoked by the user with `/name`. All 8 built-in commands (help, clear, step, model, session, compact, export, quit) use this same API.

```go
app.RegisterCommand(&ext.Command{
    Name:        "note",
    Description: "Save a quick note",
    Handler: func(args string, a *ext.App) error {
        // args contains everything after "/note "
        a.ShowMessage("Note saved: " + args)
        return nil
    },
})
```

### Command Handler API

Commands interact with the TUI through `ext.App` methods:

| Method | Effect |
|--------|--------|
| `a.ShowMessage(text)` | Display a message in the conversation |
| `a.RequestQuit()` | Signal the TUI to exit |
| `a.ShowPicker(title, items, onSelect)` | Open a modal picker |
| `a.SetStatus(key, text)` | Update the status bar |
| `a.ConversationMessages()` | Get conversation history |
| `a.SetConversationMessages(msgs)` | Replace conversation history |
| `a.ToggleStepMode()` | Toggle step-by-step tool approval |

## Keyboard Shortcuts

Built-in shortcuts (Ctrl+P for model selector, Ctrl+S for session picker) register through this API. Add your own:

```go
app.RegisterShortcut(&ext.Shortcut{
    Key:         "ctrl+g",
    Description: "Show git status",
    Handler: func(a *ext.App) error {
        a.SendMessage("show me git status")
        return nil
    },
})
```

Key format: `ctrl+<letter>`, `alt+<letter>`, `ctrl+alt+<letter>`.

## Prompt Sections

Add instructions to the system prompt that the LLM follows:

```go
app.RegisterPromptSection(ext.PromptSection{
    Title:   "Code Style",
    Content: "Always use table-driven tests.\nPrefer errgroup over sync.WaitGroup.",
    Order:   10, // lower = earlier in prompt
})
```

Prompt sections appear after the base identity and before tool hints. Users can also customize the base prompt via `~/.config/piglet/prompt.md` or `systemPrompt` in config.yaml.

## Interceptors

Interceptors wrap tool execution with before/after hooks. Use them for logging, security checks, or input transformation.

```go
app.RegisterInterceptor(ext.Interceptor{
    Name:     "audit-log",
    Priority: 1000,
    Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
        log.Printf("tool call: %s", toolName)
        return true, args, nil // allow, pass args through
    },
})
```

Priority controls execution order (higher runs first). Use 2000+ for security, 1000+ for logging.

## Examples

See [`examples/extensions/`](../examples/extensions/) for working code:

- **quicknotes/** — Adds a `/note` command for saving timestamped notes
- **git-tool/** — Adds `git_status` and `git_diff` as LLM-callable tools
