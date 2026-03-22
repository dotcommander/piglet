# Piglet

Extension-first TUI coding assistant. Go 1.26.x · Module: `github.com/dotcommander/piglet`

## Architecture: Extension-First (Extension-Only If We Could)

Piglet's core is deliberately minimal — an agent loop, streaming, and types. **Everything else is an extension.** The binary ships with a small set of compiled-in extensions (`tool/`, `command/`, `prompt/`). Seven extensions run as standalone binaries via JSON-RPC over stdin/stdout, built from source in this repo and installed to `~/.config/piglet/extensions/`.

**The rule**: New functionality MUST register through `ext.App`. Never wire behavior directly into `core/` or `cmd/piglet/main.go`. The architecture test (`ext/architecture_test.go`) enforces dependency boundaries — violations break the build.

### Dependency Direction (enforced by test)

```
core/       → imports NOTHING from piglet (agent loop, streaming, types)
ext/        → core/ only (registration surface)
tool/, command/, memory/, prompt/ → ext/, core/ (extensions — same API as external)
tui/, cmd/  → anything (wiring layer)
```

### What Registers Through ext.App

**Compiled-in** (ship with the binary):

| Kind | Count | Source | API |
|------|-------|--------|-----|
| Tools | 7 | `tool/` | `RegisterTool` |
| Commands | 18 | `command/` | `RegisterCommand` |
| Shortcuts | 2 | `command/` | `RegisterShortcut` |
| Status sections | 5 | `command/` | `RegisterStatusSection` |
| Prompt sections | 4 | `prompt/` (behavior, selfknowledge, gitcontext, projectdocs) | `RegisterPromptSection` |
| Compactor | 1 | `command/` | `RegisterCompactor` |

**External** (standalone Go binaries via JSON-RPC, built with `make extensions`):

| Extension | Binary | Registers |
|-----------|--------|-----------|
| `safeguard` | `safeguard/cmd/` | 1 interceptor |
| `rtk` | `rtk/cmd/` | 1 interceptor, 1 prompt section |
| `autotitle` | `autotitle/cmd/` | 1 event handler |
| `clipboard` | `clipboard/cmd/` | 1 tool, 1 shortcut |
| `skill` | `skill/cmd/` | 2 tools, 1 command, 1 prompt section, 1 message hook |
| `memory` | `memory/cmd/` | 3 tools, 1 command, 1 prompt section |
| `subagent` | `subagent/cmd/` | 1 tool |

All extensions (compiled-in and external) use the same `ext.App` API. External extensions communicate via JSON-RPC v2 over stdin/stdout using the Go SDK (`sdk/go/`).

### Five Primitives

Every extension capability reduces to five orchestrator primitives:

| Primitive | What it does | ext.App API |
|-----------|-------------|-------------|
| **Inject** | Put text into the conversation | `RegisterPromptSection` (static, at start) or tool result (dynamic, mid-conversation) |
| **Intercept** | Modify or block requests/responses passing through | `RegisterInterceptor` (before/after hooks on tool calls) |
| **React** | Respond to triggers | `RegisterCommand` (user input), `RegisterTool` (model-initiated) |
| **Hook** | React to user messages before the LLM sees them | `RegisterMessageHook` (ephemeral turn-scoped context injection) |
| **Observe** | React to agent lifecycle events | `RegisterEventHandler` (EventAgentEnd, EventTurnEnd, etc.) |

All extensions map to these primitives — no special access:

| Extension | Primitive | How | Where |
|-----------|-----------|-----|-------|
| `prompt/behavior.go` | Inject | Prompt section loads `behavior.md` | compiled-in |
| `prompt/selfknowledge.go` | Inject | Prompt section with runtime facts | compiled-in |
| `command/` | React | Commands respond to user slash input | compiled-in |
| `memory/` | Inject + React | Prompt section + tools | external |
| `safeguard/` | Intercept | Before hook blocks dangerous bash | external |
| `rtk/` | Inject + Intercept | Prompt section + bash rewriter | external |
| `skill/` | Inject + React + Hook | Tools + message hook | external |
| `subagent/` | React | Dispatch tool delegates to sub-agents | external |
| `clipboard/` | React | Tool + shortcut for images | external |
| `autotitle/` | Observe | Event handler for session titles | external |

**New features should use existing primitives, not add new ones.**

### Extension Registration Pattern

Compiled-in packages follow the same `Register(app)` pattern:

```go
// tool/register.go, command/builtins.go, prompt/*.go
func Register(app *ext.App) {
    app.RegisterTool(...)
    app.RegisterCommand(...)
    app.RegisterPromptSection(...)
}
```

External extensions use the Go SDK (`sdk/go/`):

```go
// safeguard/cmd/main.go, memory/cmd/main.go, etc.
func main() {
    e := sdk.New("name", "0.1.0")
    e.RegisterTool(sdk.ToolDef{...})
    e.RegisterInterceptor(sdk.InterceptorDef{...})
    e.Run() // JSON-RPC loop over stdin/stdout
}
```

`cmd/piglet/main.go` creates `ext.NewApp()`, calls compiled-in `Register()` functions, loads external extensions via `external.LoadAll()`, then passes `app` to the agent and TUI.

## Layout

```
cmd/piglet/    Wiring layer — creates ext.App, calls Register(), loads externals, runs TUI
core/          Agent loop, streaming, types. Imports nothing from piglet.
ext/           Registration surface (ext.App) — the central API
  app.go       Struct, NewApp, Bind, action queue
  registry.go  Register* methods, interceptor chain
  queries.go   Getter methods (Tools, Commands, etc.)
  runtime.go   Agent facade (SendMessage, Provider, etc.)
  domain.go    Session/model domain methods
  events.go    Event handler dispatch (Observe primitive)
  external/    JSON-RPC v2 bridge for external extensions (Go/TypeScript/Python)
sdk/go/        Go Extension SDK — JSON-RPC client for building external extensions
tool/          7 compiled-in tools
command/       18 compiled-in commands, 5 status sections, 2 shortcuts
prompt/        System prompt builder + 4 compiled-in prompt sections
config/        Settings (YAML), auth (JSON)
provider/      OpenAI, Anthropic, Google streaming providers
session/       JSONL conversation persistence, compaction
tui/           Bubble Tea v2 UI

# External extensions (standalone binaries, source in-repo):
safeguard/     Dangerous command blocking — 1 interceptor
  cmd/         Binary entry point + manifest.yaml
rtk/           Token-optimized bash rewriting — 1 interceptor, 1 prompt section
  cmd/         Binary entry point + manifest.yaml
autotitle/     Session title generation — 1 event handler
  cmd/         Binary entry point + manifest.yaml
clipboard/     Clipboard image reading — 1 tool, 1 shortcut (ctrl+v)
  cmd/         Binary entry point + manifest.yaml
skill/         On-demand methodology loading — 2 tools, 1 command, 1 prompt section, 1 message hook
  cmd/         Binary entry point + manifest.yaml
memory/        Per-project persistent memory — 3 tools, 1 command, 1 prompt section
  cmd/         Binary entry point + manifest.yaml
subagent/      Sub-agent delegation — 1 tool (dispatch)
  cmd/         Binary entry point + manifest.yaml
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

## Extensions

```bash
make extensions              # Build + install all 7 to ~/.config/piglet/extensions/
make extensions-safeguard    # Build a single extension
```

Without `make extensions`, piglet starts as a minimal agent (7 tools, 18 commands, no interceptors/events). With extensions installed, full functionality is available (14 tools, 20 commands, interceptors, shortcuts, event handlers, message hooks).

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
