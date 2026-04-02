# Building Extensions

- [Five Primitives](#five-primitives)
- [Quick Start](#quick-start)
- [Go SDK](#go-sdk)
- [TypeScript SDK](#typescript-sdk)
- [Manifest Format](#manifest-format)
- [Registration API](#registration-api)
- [Runtime API](#runtime-api)
- [Agent Events](#agent-events)
- [Actions](#actions)
- [Extension Packs](#extension-packs)
- [Writing an SDK for Another Language](#writing-an-sdk-for-another-language)
- [Examples](#examples)

## Five Primitives

Every extension capability maps to one of five primitives:

| Primitive | What It Does | API |
|-----------|-------------|-----|
| **Inject** | Put text into the conversation | `RegisterPromptSection` (static) or tool result (dynamic) |
| **Intercept** | Modify or block requests/responses | `RegisterInterceptor` (before/after hooks on tool calls) |
| **React** | Respond to triggers | `RegisterCommand` (user input), `RegisterTool` (model-initiated) |
| **Hook** | Pre-process user messages | `RegisterMessageHook` (ephemeral context injection) |
| **Observe** | React to lifecycle events | `RegisterEventHandler` (agent start/end, turn end, etc.) |

New features should use existing primitives, not add new ones.

## Quick Start

The fastest way to scaffold an extension (requires pack-core):

```
/ext-init my-extension
```

This creates `~/.config/piglet/extensions/my-extension/` with a working scaffold. Restart piglet to load it.

## Go SDK

The Go SDK ([`github.com/dotcommander/piglet/sdk`](https://github.com/dotcommander/piglet/sdk)) is what all official extensions use.

### Minimal Example

```go
package main

import (
    "context"
    sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
    e := sdk.New("my-extension", "0.1.0")

    e.RegisterTool(sdk.ToolDef{
        Name:        "hello",
        Description: "Say hello to someone",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "name": map[string]any{"type": "string", "description": "Who to greet"},
            },
            "required": []string{"name"},
        },
        Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
            name := args["name"].(string)
            return sdk.TextResult("Hello, " + name + "!"), nil
        },
    })

    e.Run() // Blocks — runs JSON-RPC loop over FD 3/4
}
```

### Registration Methods

| Method | Description |
|--------|-------------|
| `e.RegisterTool(def)` | Register an LLM-callable tool |
| `e.RegisterCommand(def)` | Register a slash command |
| `e.RegisterPromptSection(def)` | Add a system prompt section |
| `e.RegisterInterceptor(def)` | Register before/after tool hooks |
| `e.RegisterEventHandler(def)` | Observe agent lifecycle events |
| `e.RegisterShortcut(def)` | Register a keyboard shortcut |
| `e.RegisterMessageHook(def)` | Hook user messages before the LLM |
| `e.RegisterInputTransformer(def)` | Rewrite or consume user input |
| `e.RegisterCompactor(def)` | Register a conversation compactor |
| `e.RegisterProvider(api)` | Declare streaming provider capability |
| `e.OnProviderStream(handler)` | Handle `provider/stream` requests |

### Runtime Methods

Available after the extension is initialized:

| Method | Description |
|--------|-------------|
| `e.CWD()` | Working directory |
| `e.ConfigDir()` | Extension's namespaced config directory |
| `e.Notify(msg)` | TUI notification (info level) |
| `e.NotifyWarn(msg)` | TUI notification (warn level) |
| `e.NotifyError(msg)` | TUI notification (error level) |
| `e.ShowMessage(text)` | Display text in the conversation |
| `e.SendMessage(content)` | Queue a follow-up user message |
| `e.Steer(content)` | Interrupt and inject a steering message |
| `e.ShowOverlay(key, title, content, anchor, width)` | Show a TUI overlay |
| `e.CloseOverlay(key)` | Close a TUI overlay |
| `e.SetWidget(key, placement, content)` | Set a TUI widget |
| `e.Log(level, msg)` | Log to the host |

### Host Methods

Call back into the host for LLM access and state queries:

| Method | Description |
|--------|-------------|
| `e.CallHostTool(ctx, name, args)` | Execute a registered tool |
| `e.Chat(ctx, req)` | Single-turn LLM call |
| `e.Agent(ctx, req)` | Full agent loop with tools |
| `e.Sessions(ctx)` | List available sessions |
| `e.ExtInfos(ctx)` | List loaded extensions |
| `e.AuthGetKey(ctx, provider)` | Get API key from host auth |
| `e.ConfigGet(ctx, keys...)` | Read host config values |
| `e.Subscribe(ctx, topic, fn)` | Subscribe to inter-extension events |

### Lifecycle Hooks

```go
e.OnInit(func(e *sdk.Extension) {
    // Called after initialize handshake
    // e.CWD() and e.ConfigDir() are now available
})

e.OnInitAppend(func(e *sdk.Extension) {
    // Like OnInit, but appends (doesn't replace)
    // Use when multiple packages need init hooks
})
```

## TypeScript SDK

```typescript
import { piglet } from "@piglet/sdk";

piglet.setInfo("my-extension", "0.1.0");

piglet.registerTool({
  name: "hello",
  description: "Say hello",
  parameters: {
    type: "object",
    properties: {
      name: { type: "string", description: "Who to greet" },
    },
    required: ["name"],
  },
  execute: async (args) => {
    return { text: `Hello, ${args.name}!` };
  },
});

piglet.registerCommand({
  name: "wave",
  description: "Wave at someone",
  handler: async (args) => {
    piglet.notify(`Waving at ${args}!`);
  },
});

piglet.registerPromptSection({
  title: "My Guidelines",
  content: "Always be concise and precise.",
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
| `piglet.showMessage(text)` | Display text in the conversation |
| `piglet.log(level, message)` | Log to the host |
| `piglet.getCwd()` | Get the working directory |

## Manifest Format

Every external extension directory must contain a `manifest.yaml`:

```yaml
name: my-extension           # Required — unique identifier
version: 0.1.0               # Optional — semver
runtime: bun                  # Required — how to run it
entry: index.ts               # Optional — entry point (omit for compiled binaries)
capabilities:                 # Optional — descriptive only
  - tools
  - commands

defaults:                     # Optional — seed files on first install
  - src: ./config.json
    dest: ~/.config/piglet/extensions/my-extension/config.json
```

### Supported Runtimes

| Runtime Value | Command Executed |
|---------------|-----------------|
| `bun` | `bun run <entry>` |
| `node` | `node <entry>` |
| `deno` | `deno run --allow-all <entry>` |
| `python` | `python3 <entry>` |
| `./binary` | `./binary` directly |
| `/absolute/path` | `/absolute/path <entry>` |

For compiled Go extensions, set `runtime` to the binary path and omit `entry`:

```yaml
name: pack-core
version: 0.1.0
runtime: ./pack-core
capabilities:
  - commands
```

### Default Files

The `defaults` section seeds configuration files on first install:

```yaml
defaults:
  - src: ./default-config.yaml
    dest: ~/.config/piglet/extensions/my-extension/config.yaml
```

Files are only copied if the destination doesn't already exist.

## Registration API

### Tools

Tools are functions the LLM can call autonomously:

```go
e.RegisterTool(sdk.ToolDef{
    Name:        "weather",
    Description: "Get current weather for a city",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{"type": "string", "description": "City name"},
        },
        "required": []string{"city"},
    },
    PromptHint: "Check weather before recommending outdoor activities",
    Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
        city := args["city"].(string)
        // ... fetch weather ...
        return sdk.TextResult("72F, sunny in " + city), nil
    },
})
```

#### Tool Options

| Field | Type | Description |
|-------|------|-------------|
| `Name` | string | Unique tool name (required) |
| `Description` | string | What the tool does (shown to LLM) |
| `Parameters` | any | JSON Schema for arguments |
| `PromptHint` | string | One-liner added to the system prompt |
| `PromptGuides` | []string | Bullet points added to the system prompt |
| `BackgroundSafe` | bool | Safe for background agent (read-only operations) |
| `Deferred` | bool | Only send name+description to API; full schema on demand |
| `InterruptBehavior` | string | `"block"` to keep running when user steers (default: cancel) |
| `Execute` | func | The implementation |

### Commands

Slash commands invoked by the user:

```go
e.RegisterCommand(sdk.CommandDef{
    Name:        "note",
    Description: "Save a quick note",
    Immediate:   false,          // true = can run during streaming
    Handler: func(ctx context.Context, args string) error {
        e.ShowMessage("Saved: " + args)
        return nil
    },
})
```

Setting `Immediate: true` allows the command to run even while the agent is streaming. Use this for commands that don't modify conversation state (e.g., toggling UI elements).

### Interceptors

Before/after hooks on tool execution:

```go
e.RegisterInterceptor(sdk.InterceptorDef{
    Name:     "safeguard",
    Priority: 2000,    // Higher = runs first
    Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
        if toolName == "bash" && isDangerous(args) {
            return false, nil, nil  // Block execution
        }
        return true, args, nil      // Allow, pass args through
    },
    After: func(ctx context.Context, toolName string, result any) (any, error) {
        return result, nil           // Pass result through
    },
})
```

**Priority guidelines:**

| Range | Use Case |
|-------|----------|
| 2000+ | Security (safeguard, permission checks) |
| 1000+ | Logging, auditing |
| 500+ | Transformation, enrichment |
| 0+ | Default |

### Event Handlers

Observe agent lifecycle events:

```go
e.RegisterEventHandler(sdk.EventHandlerDef{
    Name:     "usage-tracker",
    Priority: 100,
    Events:   []string{"EventTurnEnd"},  // Filter to specific events (nil = all)
    Handle: func(ctx context.Context, eventType string, data json.RawMessage) *sdk.Action {
        // Process event, optionally return an action
        return nil
    },
})
```

See [Agent Events](#agent-events) for the full event list.

### Message Hooks

Pre-process user messages before the LLM sees them:

```go
e.RegisterMessageHook(sdk.MessageHookDef{
    Name:     "context-injector",
    Priority: 100,    // Lower = runs first
    OnMessage: func(ctx context.Context, msg string) (string, error) {
        // Return extra context to inject (ephemeral, one turn only)
        return "Current time: " + time.Now().Format(time.RFC3339), nil
    },
})
```

Return an empty string to inject nothing. The returned text is appended as ephemeral context for that turn only.

### Input Transformers

Rewrite or consume user input before it reaches the agent:

```go
e.RegisterInputTransformer(sdk.InputTransformerDef{
    Name:     "shorthand-expander",
    Priority: 100,
    Transform: func(ctx context.Context, input string) (string, bool, error) {
        if input == "!s" {
            return "show me git status", false, nil  // Rewrite, don't consume
        }
        return input, false, nil  // Pass through
    },
})
```

If `handled` (second return) is `true`, the input is consumed and not passed to the agent.

### Prompt Sections

Static text injected into the system prompt:

```go
e.RegisterPromptSection(sdk.PromptSectionDef{
    Title:     "Code Style",
    Content:   "Always write table-driven tests.\nPrefer errgroup over sync.WaitGroup.",
    Order:     50,    // Lower = earlier in prompt
    TokenHint: 100,   // Estimated token count (for budget display)
})
```

### Compactors

Custom conversation compaction strategy:

```go
e.RegisterCompactor(sdk.CompactorDef{
    Name: "llm-compactor",
    Compact: func(ctx context.Context, messages []sdk.CompactMessage) ([]sdk.CompactMessage, error) {
        // Summarize old messages, keep recent ones
        // ...
        return compacted, nil
    },
})
```

Only one compactor is active at a time (last registration wins).

## Runtime API

### Notifications

```go
e.Notify("Extension loaded successfully")        // Info level
e.NotifyWarn("Config file missing, using defaults")
e.NotifyError("Failed to connect to service")
```

Notifications appear as toast messages in the TUI for ~3 seconds.

### Messages

```go
e.ShowMessage("Here's some information")     // Display in conversation (not sent to LLM)
e.SendMessage("analyze the code in main.go") // Queue as user message (sent to LLM)
e.Steer("focus on the error handling")       // Interrupt current turn and redirect
```

### Overlays and Widgets

```go
// Show an overlay panel
e.ShowOverlay("my-panel", "Panel Title", "Content here", "bottom", "50%")
e.CloseOverlay("my-panel")

// Set a persistent widget
e.SetWidget("my-status", "above-input", "Current task: analyzing code")
e.SetWidget("my-status", "", "")  // Clear widget
```

Widget placements: `"above-input"` or `"below-status"`.

### Inter-Extension Communication

Extensions can communicate via a pub/sub event bus:

```go
// Publisher
e.Publish("memory:updated", map[string]any{"key": "project-context"})

// Subscriber
e.Subscribe(ctx, "memory:updated", func(data any) {
    // Handle event
})
```

## Agent Events

The 17 agent events that extensions can observe:

| Event | When | Key Payload Fields |
|-------|------|--------------------|
| `EventAgentStart` | Agent loop begins | — |
| `EventAgentEnd` | Agent loop finished | `Messages` |
| `EventSessionLoad` | Pre-loaded messages exist | `MessageCount` |
| `EventAgentInit` | Agent configured, before first call | `ToolCount` |
| `EventPromptBuild` | System prompt assembled | `System` |
| `EventMessagePre` | User message about to enter history | `Content` |
| `EventTurnStart` | New turn begins | — |
| `EventStreamDelta` | Incremental streaming chunk | `Kind`, `Index`, `Delta` |
| `EventStreamDone` | LLM finished streaming | `Message` |
| `EventToolStart` | Tool execution begins | `ToolCallID`, `ToolName`, `Args` |
| `EventToolUpdate` | Partial tool result | `ToolCallID`, `ToolName`, `Partial` |
| `EventToolEnd` | Tool execution finished | `ToolCallID`, `ToolName`, `Result`, `IsError` |
| `EventTurnEnd` | Turn completed | `Assistant`, `ToolResults` |
| `EventRetry` | Retrying after error | `Attempt`, `Max`, `DelayMs`, `Error` |
| `EventMaxTurns` | Max turns reached | `Count`, `Max` |
| `EventStepWait` | Paused for step approval | `ToolCallID`, `ToolName`, `Args` |
| `EventCompact` | Auto-compaction occurred | `Before`, `After`, `TokensAtCompact` |

## Actions

Event handlers and shortcut handlers can return actions to request TUI operations:

| Action | Description |
|--------|-------------|
| `ActionNotify{Message, Level}` | Show a notification |
| `ActionShowMessage{Text}` | Display text in conversation |
| `ActionSetStatus{Key, Text}` | Update status bar |
| `ActionSendMessage{Content}` | Queue a user message |
| `ActionQuit{}` | Exit piglet |
| `ActionRunAsync{Fn}` | Run a function in a goroutine |
| `ActionShowPicker{...}` | Open a modal picker |
| `ActionSetWidget{...}` | Set a TUI widget |
| `ActionShowOverlay{...}` | Show an overlay panel |
| `ActionCloseOverlay{Key}` | Close an overlay |

Return `nil` from a handler to take no action.

## Extension Packs

To consolidate multiple extensions into a single binary:

```go
// packs/my-pack/main.go
package main

import (
    sdk "github.com/dotcommander/piglet/sdk"
    "my-pack/extA"
    "my-pack/extB"
)

func main() {
    e := sdk.New("my-pack", "0.1.0")
    extA.Register(e)
    extB.Register(e)
    e.Run()
}
```

Each constituent extension exports a `Register(e *sdk.Extension)` function. The pack wires them together in `main()`.

Official packs wrap each `Register` call in panic recovery so one extension's failure doesn't crash the pack.

## Writing an SDK for Another Language

The wire protocol is newline-delimited JSON-RPC 2.0 over file descriptors:

- **FD 3**: host writes, extension reads
- **FD 4**: extension writes, host reads
- Falls back to stdin/stdout if FD 3/4 are not available

### Handshake

1. Host sends `initialize` request with `{ protocolVersion, cwd, configDir }`
2. Extension sends `register/*` notifications for each capability
3. Extension responds to `initialize` with `{ name, version }`

### Runtime

4. Host sends requests as needed: `tool/execute`, `command/execute`, `interceptor/before`, `interceptor/after`, `event/dispatch`, `shortcut/handle`, `messageHook/onMessage`
5. Extension can send notifications back: `host/notify`, `host/showMessage`, `host/log`
6. Host sends `shutdown` when done

For the full protocol specification, see [Protocol](protocol.md).

## Examples

Working examples in the source tree at [`examples/extensions/`](../examples/extensions/):

| Example | Language | What It Adds |
|---------|----------|-------------|
| `quicknotes/` | Go | `/note` command for timestamped notes |
| `git-tool/` | Go | `git_status` and `git_diff` tools |
| `ts-hello/` | TypeScript | `hello_world` tool and `/wave` command |
