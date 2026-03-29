# Extensions

Extensions add custom functionality to piglet: tools, slash commands, keyboard shortcuts, prompt sections, interceptors, event handlers, and message hooks. See [extensions-vs-mcp.md](extensions-vs-mcp.md) for guidance on when to use extensions vs MCP servers.

Piglet ships with a small compiled-in set of core tools and commands. Additional extensions run as standalone binaries via JSON-RPC from [`piglet-extensions`](https://github.com/dotcommander/piglet-extensions), adding tools, commands, shortcuts, interceptors, event handlers, message hooks, and dynamic MCP tools. Run `/extensions` to see what's loaded.

## Quick Start

The fastest way to create an extension:

```
/ext-init my-extension
```

This creates `~/.config/piglet/extensions/my-extension/` with a ready-to-run scaffold. Restart piglet to load it. (Requires the `scaffold` extension from [piglet-extensions](https://github.com/dotcommander/piglet-extensions).)

To see what's loaded: `/extensions`

## Extension Packs

Official extensions are consolidated into packs — single binaries that register capabilities from multiple logical extensions. This reduces startup from ~3 seconds (32 processes) to under 1 second (6 processes).

| Pack | Contains | Capabilities |
|------|----------|-------------|
| **pack-context** | memory, skill, gitcontext, behavior, prompts, session-tools, inbox | 6 tools, 6 commands, 5 prompt sections, 1 compactor, 3 event handlers, 1 message hook |
| **pack-code** | lsp, repomap, sift, plan, suggest | 10 tools, 1 command, 4 prompt sections, 2 interceptors, 2 event handlers |
| **pack-agent** | safeguard, rtk, autotitle, clipboard, subagent, provider, loop | 2 tools, 3 commands, 2 prompt sections, 2 interceptors, 1 shortcut, 1 event handler, stream providers |
| **pack-core** | admin, export, extensions-list, undo, scaffold, background | 8 commands |
| **pack-workflow** | pipeline, bulk, webfetch, cache, usage, modelsdev | 7 tools, 3 commands, 3 prompt sections, 1 event handler |
| **mcp** | mcp | Dynamic tools, 1 command, 1 prompt section (standalone) |

Packs use the same `ext.App` API as individual extensions. Each constituent extension keeps its own package — only the binary entry points are merged. Individual extensions remain independently buildable for development and testing.

### Pack Architecture

Each pack binary:
1. Creates one `sdk.Extension` named `pack-<name>`
2. Calls `Register(e)` from each constituent extension package
3. Wraps each `Register` in panic recovery — one extension's failure doesn't crash the pack
4. Runs the JSON-RPC event loop

```go
// packs/core/main.go
func main() {
    e := sdk.New("pack-core", "0.1.0")
    safety.Register(e, "admin", admin.Register)
    safety.Register(e, "export", export.Register)
    // ...
    e.Run()
}
```

To add a new extension to a pack, export `Register(e *sdk.Extension)` from the extension's library package and wire it in the pack's `main.go`.

## External Extensions (Go, TypeScript, Python, etc.)

External extensions run as child processes and communicate via JSON-RPC v2 over **FD 3/4** (not stdin/stdout — those are free for the extension's own use). They can be written in any language — Go, TypeScript, Python, Ruby, etc. Piglet's own extensions are external Go binaries in the [`piglet-extensions`](https://github.com/dotcommander/piglet-extensions) repo.

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
runtime: bun          # bun, node, deno, python, or path to binary
entry: index.ts       # entry point (omit for compiled Go binaries)
capabilities:
  - tools
  - commands
  - interceptors
  - shortcuts
```

Supported runtimes:

| Runtime | Command |
|---------|---------|
| `bun` | `bun run <entry>` |
| `node` | `node <entry>` |
| `deno` | `deno run --allow-all <entry>` |
| `python` | `python3 <entry>` |
| `./binary` | `./binary` (compiled Go, no entry needed) |
| `/path/to/bin` | `/path/to/bin <entry>` |

For compiled Go binaries, set `runtime` to the binary path and omit `entry`:

```yaml
name: safeguard
version: 0.1.0
runtime: ./safeguard
capabilities:
  - interceptors
```

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

### TypeScript SDK API

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

### Go SDK

The Go SDK ([`piglet/sdk`](https://github.com/dotcommander/piglet/sdk)) provides the same capabilities for compiled Go extensions. All of piglet's own extensions (in [`piglet-extensions`](https://github.com/dotcommander/piglet-extensions)) use this SDK.

```go
package main

import (
    "context"
    sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
    e := sdk.New("my-extension", "0.1.0")

    e.RegisterTool(sdk.ToolDef{
        Name:        "my_tool",
        Description: "Does something useful",
        Parameters:  map[string]any{"type": "object", "properties": map[string]any{
            "input": map[string]any{"type": "string"},
        }},
        Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
            return sdk.TextResult("done"), nil
        },
    })

    e.RegisterInterceptor(sdk.InterceptorDef{
        Name:     "my-guard",
        Priority: 1000,
        Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
            return true, args, nil // allow
        },
    })

    e.RegisterEventHandler(sdk.EventHandlerDef{
        Name:   "my-observer",
        Events: []string{"EventAgentEnd"},
        Handle: func(ctx context.Context, eventType string, data json.RawMessage) *sdk.Action {
            return sdk.ActionNotify("Agent finished")
        },
    })

    e.Run() // JSON-RPC loop over FD 3/4
}
```

Go SDK API:

| Method | Description |
|--------|-------------|
| `sdk.New(name, version)` | Create extension |
| `e.RegisterTool(def)` | Register an LLM-callable tool |
| `e.RegisterCommand(def)` | Register a slash command |
| `e.RegisterPromptSection(def)` | Add a system prompt section |
| `e.RegisterInterceptor(def)` | Register before/after tool hooks |
| `e.RegisterEventHandler(def)` | Observe agent lifecycle events |
| `e.RegisterShortcut(def)` | Register keyboard shortcut |
| `e.RegisterMessageHook(def)` | Hook user messages before LLM sees them |
| `e.OnInit(fn)` | Callback after initialize (receives CWD) |
| `e.Notify(msg)` | Show a TUI notification |
| `e.ShowMessage(text)` | Display a message in conversation |
| `e.Log(level, msg)` | Log to the host |
| `e.CWD()` | Get the working directory |
| `e.RegisterCompactor(def)` | Register a conversation compactor |
| `e.RegisterProvider(api)` | Declare provider streaming capability |
| `e.OnProviderStream(handler)` | Handle provider/stream requests from host |
| `e.SendProviderDelta(reqID, kind, idx, delta, tool)` | Send streaming delta to host |
| `e.SendMessage(content)` | Inject a user message into the agent loop |
| `e.Steer(content)` | Interrupt current turn with a steering message |
| `e.ConfigGet(ctx, keys...)` | Read host config values |
| `e.ConfigReadExtension(ctx, name)` | Read extension config file |
| `e.AuthGetKey(ctx, provider)` | Get API key from host auth |
| `e.Chat(ctx, req)` | Single-turn LLM call via host |
| `e.RunAgent(ctx, req)` | Full agent loop via host |
| `e.Run()` | Start the JSON-RPC loop |

### Writing an SDK for Another Language

The protocol is newline-delimited JSON-RPC 2.0 over FD 3 (host→extension) and FD 4 (extension→host). See [docs/protocol.md](protocol.md) for the full message specification. The flow:

1. Host sends `initialize` request with `{ protocolVersion, cwd }`
2. Extension sends `register/*` notifications (tool, command, promptSection, interceptor, eventHandler, shortcut, messageHook)
3. Extension responds to `initialize` with `{ name, version }`
4. Host sends requests as needed: `tool/execute`, `command/execute`, `interceptor/before`, `interceptor/after`, `event/dispatch`, `shortcut/handle`, `messageHook/onMessage`
5. Host sends `shutdown` when done

## Compiled-In Extensions

A small set of extensions are compiled directly into the binary for startup performance:

```go
// tool/register.go, command/builtins.go, prompt/*.go
func Register(app *ext.App) {
    app.RegisterTool(...)
    app.RegisterCommand(...)
}
```

These use the same `ext.App` API as external extensions. See `tool/`, `command/`, and `prompt/` for the compiled-in set.

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

Commands are invoked by the user with `/name`. All built-in commands use this same API.

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
    Handler: func(a *ext.App) (ext.Action, error) {
        a.SendMessage("show me git status")
        return nil, nil
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

1. Compiled-in tools (`tool/`)
2. Compiled-in commands, shortcuts, prompt sections (`command/`, `prompt/`)
3. External extensions from `~/.config/piglet/extensions/` (alphabetical by directory name)
   - Official extensions from [piglet-extensions](https://github.com/dotcommander/piglet-extensions)
   - User extensions: anything else in the extensions directory

Later registrations overwrite earlier ones for the same name.

## Building Extensions

Official extensions live in [`piglet-extensions`](https://github.com/dotcommander/piglet-extensions):

```bash
git clone https://github.com/dotcommander/piglet-extensions
cd piglet-extensions
make extensions              # Build + install all to ~/.config/piglet/extensions/
make extensions-safeguard    # Build a single extension
```

Without extensions installed, piglet runs as a minimal agent with only the compiled-in tools and commands.

## Examples

See [`examples/extensions/`](../examples/extensions/) for working code:

- **quicknotes/** — Go: Adds a `/note` command for saving timestamped notes
- **git-tool/** — Go: Adds `git_status` and `git_diff` as LLM-callable tools
- **ts-hello/** — TypeScript: Adds a `hello_world` tool and `/wave` command
