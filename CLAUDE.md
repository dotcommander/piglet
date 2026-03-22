# Piglet

Extension-first TUI coding assistant. Go 1.26.x · Module: `github.com/dotcommander/piglet`

## Architecture: Extension-First (Extension-Only If We Could)

Piglet's core is deliberately minimal — an agent loop, streaming, and types. **Everything else is an extension.** Built-in packages (`tool/`, `command/`, `prompt/`, `memory/`) register through the exact same `ext.App` API that external extensions use. They ship with the binary, but they have no special access. If Go supported true plugin isolation cleanly, this would be extension-only.

**The rule**: New functionality MUST register through `ext.App`. Never wire behavior directly into `core/` or `cmd/piglet/main.go`. The architecture test (`ext/architecture_test.go`) enforces dependency boundaries — violations break the build.

### Dependency Direction (enforced by test)

```
core/       → imports NOTHING from piglet (agent loop, streaming, types)
ext/        → core/ only (registration surface)
tool/, command/, memory/, prompt/ → ext/, core/ (extensions — same API as external)
tui/, cmd/  → anything (wiring layer)
```

### What Registers Through ext.App

| Kind | Count | Source | API |
|------|-------|--------|-----|
| Tools | 13 | `tool/` (7), `memory/` (2), `skill/` (2), `subagent/` (1), `clipboard/` (1) | `RegisterTool` |
| Commands | 20 | `command/` (18), `memory/` (1), `skill/` (1) | `RegisterCommand` |
| Shortcuts | 3 | `command/` (2), `clipboard/` (1) | `RegisterShortcut` |
| Status sections | 5 | `command/` | `RegisterStatusSection` |
| Prompt sections | 5+ | `prompt/` (3), `memory/` (1), `skill/` (1) | `RegisterPromptSection` |
| Message hooks | 1+ | `skill/` (1) | `RegisterMessageHook` |
| Compactor | 1 | `command/` | `RegisterCompactor` |
| Interceptors | varies | any extension | `RegisterInterceptor` |
| Event handlers | 1 | `autotitle/` (1) | `RegisterEventHandler` |
| Renderers | 0 built-in | any extension | `RegisterRenderer` |
| Providers | 0 built-in | any extension | `RegisterProvider` |

External extensions (TypeScript, Python, Go) use identical API via JSON-RPC over stdin/stdout.

### Five Primitives

Every extension capability reduces to five orchestrator primitives:

| Primitive | What it does | ext.App API |
|-----------|-------------|-------------|
| **Inject** | Put text into the conversation | `RegisterPromptSection` (static, at start) or tool result (dynamic, mid-conversation) |
| **Intercept** | Modify or block requests/responses passing through | `RegisterInterceptor` (before/after hooks on tool calls) |
| **React** | Respond to triggers | `RegisterCommand` (user input), `RegisterTool` (model-initiated) |
| **Hook** | React to user messages before the LLM sees them | `RegisterMessageHook` (ephemeral turn-scoped context injection) |
| **Observe** | React to agent lifecycle events | `RegisterEventHandler` (EventAgentEnd, EventTurnEnd, etc.) |

All built-in extensions map to these primitives — no special access:

| Extension | Primitive | How |
|-----------|-----------|-----|
| `prompt/behavior.go` | Inject | Prompt section loads `behavior.md` at start |
| `prompt/selfknowledge.go` | Inject | Prompt section with runtime facts |
| `memory/` | Inject + React | Prompt section + tools that return/store content |
| `safeguard/` | Intercept | Before hook blocks dangerous bash commands |
| `rtk/` | Intercept | Before hook rewrites bash commands |
| `command/` | React | Commands respond to user slash input |
| `skill/` | Inject + React + Hook | Tools for on-demand loading, message hook for auto-triggering |
| `subagent/` | React | Dispatch tool delegates tasks to independent sub-agents |
| `clipboard/` | React | Tool reads images from system clipboard; shortcut attaches to next message |
| `autotitle/` | Observe | Event handler generates session title after first exchange |

**New features should use existing primitives, not add new ones.**

### Extension Registration Pattern

Every built-in package follows the same pattern:

```go
// tool/register.go, command/builtins.go, memory/register.go, prompt/*.go
func Register(app *ext.App) {
    app.RegisterTool(...)
    app.RegisterCommand(...)
    app.RegisterPromptSection(...)
}
```

`cmd/piglet/main.go` creates `ext.NewApp()`, calls each `Register()`, then passes `app` to the agent and TUI. The wiring layer has zero hardcoded tools, commands, or behaviors.

## Layout

```
cmd/piglet/    Wiring layer — creates ext.App, calls Register(), runs TUI
core/          Agent loop, streaming, types. Imports nothing from piglet.
ext/           Registration surface (ext.App) — the central API
  app.go       Struct, NewApp, Bind, action queue
  registry.go  Register* methods, interceptor chain
  queries.go   Getter methods (Tools, Commands, etc.)
  runtime.go   Agent facade (SendMessage, Provider, etc.)
  domain.go    Session/model domain methods
  events.go    Event handler dispatch (Observe primitive)
  external/    JSON-RPC bridge for TypeScript/Python/Go extensions
tool/          7 built-in tools — extension, not core
command/       18 built-in commands, 5 status sections, 2 shortcuts — extension, not core
clipboard/     Clipboard image reading — 1 tool, 1 shortcut (ctrl+v)
prompt/        System prompt builder + prompt section extensions
memory/        Per-project persistent memory — 2 tools, 1 command, 1 prompt section
skill/         On-demand methodology loading — 2 tools, 1 command, 1 prompt section, 1 message hook
subagent/      Sub-agent delegation — 1 tool (dispatch)
autotitle/     Session title generation — 1 event handler (Observe)
config/        Settings (YAML), auth (JSON)
provider/      OpenAI, Anthropic, Google streaming providers
session/       JSONL conversation persistence, compaction
tui/           Bubble Tea v2 UI
```

## Key Types

| Type | Package | Purpose |
|------|---------|---------|
| `App` | ext | Extension registration surface — the central API |
| `Agent` | core | Agent loop with streaming, tools, steering |
| `StreamProvider` | core | Interface all providers implement |
| `ToolDef` | ext | Tool definition with execution, hints, guides |
| `Command` | ext | Slash command with handler + completion |
| `Interceptor` | ext | Before/after tool hooks, priority-sorted |
| `MessageHook` | ext | Before-message hooks, priority-sorted, ephemeral context injection |
| `PromptSection` | ext | System prompt injection (title, content, order) |
| `EventHandler` | ext | Agent lifecycle event observer (Observe primitive) |

## Build & Test

```bash
go build ./...
go test -race ./... | tail -50
go vet ./...
```

## Binary

```bash
go build -o piglet ./cmd/piglet/
ln -sf ~/go/src/piglet/piglet ~/go/bin/piglet
```

## Config

- Settings: `~/.config/piglet/config.yaml`
- Auth: `~/.config/piglet/auth.json`
- System prompt: `~/.config/piglet/prompt.md` (identity — NOT a Go const)
- Behavior: `~/.config/piglet/behavior.md` (guidelines — NOT a Go const)
- Skills: `~/.config/piglet/skills/` (markdown with YAML frontmatter)
- Sessions: `~/.config/piglet/sessions/`
- Extensions: `~/.config/piglet/extensions/`
- Extension configs: `~/.config/piglet/<name>.md` (read via `config.ReadExtensionConfig`)

**All prompt content, behavioral text, and default strings live in config files above. Go code reads these files. It never contains the content. See Go workspace CLAUDE.md "Configuration Data" for the pre-flight gate.**

## Dependencies

| Package | Version |
|---------|---------|
| bubbletea | `charm.land/bubbletea/v2` v2.0.2 |
| bubbles | `charm.land/bubbles/v2` v2.0.0 |
| lipgloss | `charm.land/lipgloss/v2` v2.0.0 |
| glamour | `github.com/charmbracelet/glamour` v1.0.0 |

## Conventions

- No `init()`, no mutable package globals
- Short functions (80 lines max)
- Pointer receivers by default
- `context.Context` as first param
- `fmt.Errorf` with `%w` for error wrapping
- **New functionality = new extension, never core modification**
