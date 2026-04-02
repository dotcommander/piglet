# Architecture

- [Design Philosophy](#design-philosophy)
- [System Overview](#system-overview)
- [Core](#core)
- [Extensions (ext)](#extensions-ext)
- [Providers](#providers)
- [Sessions](#sessions)
- [Shell](#shell)
- [TUI](#tui)
- [Dependency Direction](#dependency-direction)
- [Agent Loop](#agent-loop)
- [Event Flow](#event-flow)
- [Concurrency Model](#concurrency-model)

## Design Philosophy

Piglet is **orchestration, not features.** The core is a minimal agent loop — provider-agnostic, extension-first — where every capability beyond "stream LLM, execute tools, repeat" lives in an extension.

Key principles:

- **Extension-first** — built-in tools use the same API as external extensions. Nothing is privileged.
- **Provider-agnostic** — switch between Anthropic, OpenAI, Google, xAI, Groq, OpenRouter, or local models mid-session.
- **Terminal-resident** — lives where the work happens. No browser, no IDE dependency.
- **User-owned** — all prompts, behavior, skills, and memory live as plain files in `~/.config/piglet/`.

The core is frozen at 17 events and 5 extension primitives. The answer to "how do I add X?" is always "write an extension."

## System Overview

```
┌──────────────────────────────────────────────────────┐
│  cmd/piglet/          Wiring layer — CLI, flags,     │
│                       creates App, Shell, runs TUI   │
├──────────────────────────────────────────────────────┤
│  tui/                 Bubble Tea UI — input,         │
│                       messages, status, overlays     │
├──────────────────────────────────────────────────────┤
│  shell/               Agent lifecycle — submit,      │
│                       process events, notifications  │
│                       (frontend-agnostic)            │
├──────────────────────────────────────────────────────┤
│  command/  tool/  prompt/                            │
│                       Built-in extensions            │
│                       (same API as external)         │
├──────────────────────────────────────────────────────┤
│  ext/                 Registration surface (App)     │
│  ext/external/        JSON-RPC bridge for            │
│                       external extensions            │
├──────────────────────────────────────────────────────┤
│  core/                Agent loop, streaming, types   │
│                       (imports nothing from piglet)  │
├──────────────────────────────────────────────────────┤
│  provider/            OpenAI, Anthropic, Google      │
│                       streaming implementations      │
├──────────────────────────────────────────────────────┤
│  session/             Tree-structured JSONL          │
│                       persistence                    │
├──────────────────────────────────────────────────────┤
│  config/              Settings, auth, setup          │
├──────────────────────────────────────────────────────┤
│  sdk/                 Go Extension SDK               │
│                       (standalone module)            │
└──────────────────────────────────────────────────────┘
```

## Core

**Package:** `core/`
**Imports:** Nothing from piglet.

The core is the agent loop. It streams LLM responses, executes tools, and emits events. It knows nothing about files, git, memory, code, or UI.

### Key Types

| Type | Purpose |
|------|---------|
| `Agent` | Runs the agent loop with streaming, tools, and steering |
| `StreamProvider` | Interface all providers implement |
| `Message` | Sealed interface: `UserMessage`, `AssistantMessage`, `ToolResultMessage` |
| `ContentBlock` | Sealed interface: `TextContent`, `ImageContent` |
| `ToolSchema` | Tool definition (name, description, JSON Schema parameters) |
| `Tool` | Schema + execute function |
| `ToolResult` | Execution result with content blocks |
| `Event` | Agent lifecycle events (17 total) |
| `Model` | Model metadata (ID, provider, API, context window, cost) |

### Agent API

```go
// Create and start
agent := core.NewAgent(cfg)
events := agent.Start(ctx, "user prompt")

// Control
agent.Steer(msg)              // Interrupt current turn
agent.FollowUp(msg)           // Queue for after current run
agent.Stop()                  // Cancel

// State
agent.Messages() []Message
agent.IsRunning() bool
agent.SetTools(tools)
agent.SetModel(model)
agent.SetProvider(provider)
```

## Extensions (ext)

**Package:** `ext/`
**Imports:** `core/` only.

The `ext.App` struct is the central registration surface. Every capability — tools, commands, shortcuts, interceptors, hooks, prompt sections — registers through `App`.

### Five Primitives

| Primitive | Registration | Description |
|-----------|-------------|-------------|
| Inject | `RegisterPromptSection` | Put text into the system prompt |
| Intercept | `RegisterInterceptor` | Before/after hooks on tool calls |
| React | `RegisterTool`, `RegisterCommand` | Respond to model or user triggers |
| Hook | `RegisterMessageHook` | Pre-process user messages |
| Observe | `RegisterEventHandler` | React to agent lifecycle events |

### App Lifecycle

1. `ext.NewApp(cwd)` — create the registration surface
2. Built-in `Register()` functions called — tools, commands, prompt sections
3. `external.LoadAll()` — discover and start external extensions
4. `app.Bind(agent)` — wire runtime references
5. Extensions interact via `App` methods during the session

### External Extension Bridge

External extensions run as child processes communicating via JSON-RPC v2 over file descriptors (FD 3/4). The bridge (`ext/external/`) handles:

- **Discovery** — scan `~/.config/piglet/extensions/` for `manifest.yaml`
- **Startup** — spawn process, run handshake, collect registrations
- **Proxy** — translate `App` method calls to JSON-RPC requests
- **Supervision** — crash detection, automatic restart with backoff
- **Hot reload** — graceful restart when the binary changes

## Providers

**Package:** `provider/`

Three streaming protocols implemented natively:

| Protocol | Provider | Wire Format |
|----------|----------|-------------|
| OpenAI | OpenAI, xAI, Groq, OpenRouter, Z.AI, local servers | `POST /v1/chat/completions` SSE |
| Anthropic | Anthropic | `POST /v1/messages` SSE |
| Google | Google Gemini | `POST /v1beta/models/{id}:streamGenerateContent` SSE |

Each provider implements `core.StreamProvider`:

```go
type StreamProvider interface {
    Stream(ctx context.Context, req StreamRequest) <-chan StreamEvent
}
```

The registry (`provider.Registry`) loads models from `~/.config/piglet/models.yaml`, probes local servers, and resolves model queries to concrete providers.

Extensions can register additional providers via `RegisterStreamProvider`.

## Sessions

**Package:** `session/`

Tree-structured JSONL persistence. Each session is a single `.jsonl` file where entries link via `ID`/`ParentID` to form a tree.

Key operations:

| Operation | Effect |
|-----------|--------|
| Append | Add message at current leaf |
| Branch | Move leaf to earlier entry (in-place branching) |
| Fork | Create new session file linked to this one |
| Compact | Write summarized checkpoint at current leaf |

Context is built by walking from the leaf to the root, collecting messages on the active branch path.

## Shell

**Package:** `shell/`
**Imports:** `ext/`, `core/`, `session/`

The shell manages agent lifecycle — submit, event processing, action routing, queue management, and background agents. It is a concrete struct (not an interface), so adding methods is non-breaking for all consumers.

Any frontend (Bubble Tea, a REPL, a headless test harness) creates a `Shell` and calls three methods:

| Method | Purpose |
|--------|---------|
| `Submit(input)` | Submit user input — returns a `Response` (agent started, queued, command, error) |
| `ProcessEvent(evt)` | Handle one agent event — returns true when the run is complete |
| `Notifications()` | Drain pending notifications — the frontend handles UI-relevant ones |

Shell handles headless concerns internally (session persistence, async execution, queue drain). UI-relevant actions are surfaced as `Notification` values that frontends translate into their own state changes.

### Files

| File | Responsibility |
|------|---------------|
| `shell.go` | Struct, constructor, agent wiring, accessors |
| `submit.go` | `Submit()`, `SubmitWithImage()`, command dispatch, message hooks |
| `process.go` | `ProcessEvent()`, `ProcessBgEvent()`, action drain, queue drain |
| `notify.go` | `Notification` type and `NotificationKind` enum |
| `response.go` | `Response` type and `ResponseKind` enum |
| `queue.go` | Input queue (mid-stream steering, post-run drain) |
| `background.go` | Background agent lifecycle |

## TUI

**Package:** `tui/`
**Consumes:** `shell/`

Bubble Tea v2 application. The TUI is purely a rendering and input layer — it delegates all agent lifecycle to `shell.Shell`.

Components:

| Component | Purpose |
|-----------|---------|
| `Model` | Top-level Bubble Tea model — state, update, render |
| `InputModel` | Multi-line text input with autocomplete and history |
| `MessageView` | Glamour-based markdown rendering for messages |
| `StatusBar` | Footer with extension-registered sections |
| `ModalModel` | Picker dialogs (model selector, session picker, etc.) |
| `OverlayModel` | Stacked overlay panels |

### Startup Phases

1. **Sync phase** — register built-in tools/commands (fast, ~10ms)
2. **Async phase** — load external extensions in background (~1s)
3. **TUI renders immediately** — user can type while extensions load
4. **`AgentReadyMsg`** — agent is fully configured, shell wires it via `SetAgent()`

## Dependency Direction

Enforced by `ext/architecture_test.go` — violations break the build.

```
core/                → imports NOTHING from piglet
ext/                 → core/ only
tool/, command/, prompt/ → ext/, core/
provider/, session/, config/ → core/ only (or stdlib)
shell/               → ext/, core/, session/
tui/, cmd/           → anything (wiring layer)
```

The rule: lower layers never import upper layers. Extensions and core are the boundary.

## Agent Loop

The agent loop runs in `core.Agent.run()`:

```
1. Emit EventAgentStart
2. Check for pre-loaded messages → EventSessionLoad
3. Emit EventAgentInit, EventPromptBuild
4. Append user message → EventMessagePre
5. Turn loop:
   a. Emit EventTurnStart
   b. Apply any steering messages
   c. Stream LLM response (with retry) → EventStreamDelta, EventStreamDone
   d. Extract tool calls from response
   e. Execute tools in parallel (semaphore-bounded) → EventToolStart, EventToolEnd
   f. Emit EventTurnEnd
   g. Check MaxMessages cap, trigger compaction if needed
   h. Continue if tools were called or steering messages pending
6. Wait for background compaction
7. Emit EventAgentEnd
8. Close event channel
```

### Tool Execution

Tools run in parallel with configurable concurrency (default: 10). In step mode, concurrency drops to 1 and each tool waits for user approval.

### Retry

Transient errors (rate limits, timeouts) trigger automatic retry with exponential backoff:

- Base delay: 500ms
- Max delay: 5s
- Max attempts: 3 (configurable)

### Steering

The user can interrupt a running agent with `Ctrl+C` (which cancels the current stream) or by typing a new message (which queues as a steering message). Steering messages are applied at the start of the next turn, replacing the planned continuation.

## Event Flow

Events flow from the agent through the shell and TUI to extensions:

```
Agent (core/)
  ↓ emits events on buffered channel (size 100)
Shell (shell/)
  ↓ ProcessEvent() dispatches to App.DispatchEvent()
  ↓ drains actions, persists to session, manages queue
  ↓ surfaces UI-relevant actions as Notifications
TUI (tui/)
  ↓ polls events via Bubble Tea Cmd, calls shell.ProcessEvent()
  ↓ reads shell.Notifications(), translates to TUI state
```

### Event Batching

The TUI polls the event channel and batches multiple events into a single `Update()` call. This prevents UI thrashing during rapid streaming.

### Action Queue

Extensions don't directly mutate TUI state. Instead, they return `Action` values that the TUI processes:

```
Extension handler → returns ActionNotify{"Done"}
TUI receives action → shows notification toast
```

## Concurrency Model

### Agent

- `mu` (RWMutex) protects messages, config, running state
- Tool execution uses a semaphore channel for bounded concurrency
- Compaction runs in a background goroutine with `compactWg` for shutdown synchronization
- Steering messages collected atomically during parallel tool execution

### Session

- `mu` (RWMutex) protects the in-memory tree and leaf pointer
- File writes are serialized (append-only file)

### Provider

- Each `Stream()` call spawns an independent goroutine
- Event channel must be closed by the provider (contract)
- HTTP client uses connection pooling (100 conns per host)

### Shell

- `mu` (Mutex) protects running state, queue, session, agent references
- Action drain is synchronous — called after each event
- Notification buffer is append-only, drained by frontend

### TUI

- Single-threaded Bubble Tea model (no concurrent `Update()` calls)
- Agent events arrive asynchronously on a channel, forwarded to `shell.ProcessEvent()`
- Notifications from shell translated to TUI state in `applyShellNotifications()`

### External Extensions

- Each extension is a separate OS process
- Communication via JSON-RPC over FD 3/4 (serialized per connection)
- Supervisor goroutine monitors process health
