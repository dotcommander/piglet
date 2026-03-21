# Extensions

Extensions add custom functionality to piglet: tools, slash commands, keyboard shortcuts, prompt sections, and interceptors.

All of piglet's built-in functionality (7 tools, 10 commands, 2 shortcuts) registers through the same extension API, so anything built-in can be overridden or extended.

## Quick Start

The fastest way to create an extension:

```
/ext-init my-extension
```

This creates `~/.config/piglet/extensions/my-extension/` with a ready-to-run scaffold. Restart piglet to load it.

To see what's loaded: `/extensions`

## External Extensions (TypeScript, Python, etc.)

External extensions run as child processes and communicate via JSON-RPC over stdin/stdout. They can be written in any language — TypeScript, Python, Ruby, etc.

### Directory Structure

```
~/.config/piglet/extensions/
  my-ext/
    manifest.yaml    # required: name, runtime, entry
    index.ts         # your code
    package.json     # optional: for npm/bun dependencies
```

### Manifest

Every external extension needs a `manifest.yaml`:

```yaml
name: my-extension
version: 0.1.0
runtime: bun          # bun, node, deno, python, or absolute path
entry: index.ts       # entry point relative to extension dir
capabilities:
  - tools
  - commands
```

Supported runtimes:

| Runtime | Command |
|---------|---------|
| `bun` | `bun run <entry>` |
| `node` | `node <entry>` |
| `deno` | `deno run --allow-all <entry>` |
| `python` | `python3 <entry>` |
| `/path/to/bin` | `/path/to/bin <entry>` |

### TypeScript SDK

Install the SDK in your extension directory:

```bash
cd ~/.config/piglet/extensions/my-ext
bun add @piglet/sdk
```

Then write your extension:

```typescript
import { piglet } from "@piglet/sdk";

piglet.setInfo("my-ext", "0.1.0");

// Register a tool the LLM can call
piglet.registerTool({
  name: "my_tool",
  description: "Does something useful",
  parameters: {
    type: "object",
    properties: {
      input: { type: "string", description: "The input" },
    },
    required: ["input"],
  },
  execute: async (args) => {
    return { text: `Result: ${args.input}` };
  },
});

// Register a slash command
piglet.registerCommand({
  name: "my-cmd",
  description: "Do something",
  handler: async (args) => {
    piglet.notify("Done: " + args);
  },
});

// Add to the system prompt
piglet.registerPromptSection({
  title: "My Rules",
  content: "Always be concise.",
  order: 50,
});
```

### SDK API

| Method | Description |
|--------|-------------|
| `piglet.setInfo(name, version)` | Set extension metadata |
| `piglet.registerTool(def)` | Register an LLM-callable tool |
| `piglet.registerCommand(def)` | Register a slash command |
| `piglet.registerPromptSection(def)` | Add a system prompt section |
| `piglet.notify(message)` | Show a TUI notification |
| `piglet.showMessage(text)` | Display a message in conversation |
| `piglet.log(level, message)` | Log to the host |
| `piglet.getCwd()` | Get the working directory |

### Writing an SDK for Another Language

The protocol is newline-delimited JSON-RPC 2.0 over stdin/stdout. See [`ext/external/protocol.go`](../ext/external/protocol.go) for the full message spec. The flow:

1. Host sends `initialize` request with `{ protocolVersion, cwd }`
2. Extension sends `register/tool`, `register/command`, `register/promptSection` notifications
3. Extension responds to `initialize` with `{ name, version }`
4. Host sends `tool/execute` or `command/execute` requests as needed
5. Host sends `shutdown` when done

## Go Extensions (Compiled In)

Go extensions are functions compiled into the binary:

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

## Custom Renderers

Register a renderer for a custom message type:

```go
app.RegisterRenderer("diff", func(message any, expanded bool) string {
    // Return rendered string for display
    return fmt.Sprintf("```diff\n%s\n```", message)
})
```

## Custom Providers

Register an LLM provider with its own models:

```go
app.RegisterProvider("ollama", &ext.ProviderConfig{
    BaseURL: "http://localhost:11434/v1",
    API:     core.APIOpenAI,
    Models: []core.Model{
        {ID: "llama3", Name: "Llama 3", Provider: "ollama"},
    },
})
```

## Runtime API

Beyond the command handler methods, extensions have full runtime access after binding:

| Method | Effect |
|--------|--------|
| `app.SendMessage(text)` | Queue a follow-up user message to the agent |
| `app.Steer(text)` | Inject a steering message mid-turn |
| `app.SetModel(m)` | Switch the active model |
| `app.Notify(msg)` | Show a TUI notification |
| `app.SetStatus(key, text)` | Update the status bar |
| `app.ShowMessage(text)` | Display a message in the conversation |
| `app.ShowPicker(title, items, onSelect)` | Open a modal picker |
| `app.RequestQuit()` | Signal the TUI to exit |
| `app.ConversationMessages()` | Get conversation history snapshot |
| `app.SetConversationMessages(msgs)` | Replace conversation history |
| `app.ToggleStepMode()` | Toggle step-by-step tool approval |
| `app.CWD()` | Get the working directory |
| `app.Model()` | Get the current model |

## Overriding Built-ins

External extensions load after built-ins. If you register a tool or command with the same name as a built-in, yours replaces it. For example, to replace the built-in `read` tool:

```typescript
piglet.registerTool({
  name: "read",  // same name as built-in — replaces it
  description: "My custom file reader",
  parameters: { ... },
  execute: async (args) => { ... },
});
```

The same applies to commands — register a command named `help` and your handler runs instead of the built-in.

## Extension Loading Order

1. Built-in Go tools (read, write, edit, bash, grep, find, ls)
2. Built-in Go commands (help, clear, step, model, session, compact, export, extensions, ext-init, quit)
3. External extensions from `~/.config/piglet/extensions/` (alphabetical by directory name)

Later registrations overwrite earlier ones for the same name.

## Examples

See [`examples/extensions/`](../examples/extensions/) for working code:

- **quicknotes/** — Go: Adds a `/note` command for saving timestamped notes
- **git-tool/** — Go: Adds `git_status` and `git_diff` as LLM-callable tools
- **ts-hello/** — TypeScript: Adds a `hello_world` tool and `/wave` command
