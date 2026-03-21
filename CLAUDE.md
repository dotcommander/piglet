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
| Tools | 9 | `tool/` (7), `memory/` (2) | `RegisterTool` |
| Commands | 19 | `command/` (18), `memory/` (1) | `RegisterCommand` |
| Shortcuts | 2 | `command/` | `RegisterShortcut` |
| Status sections | 5 | `command/` | `RegisterStatusSection` |
| Prompt sections | 4+ | `prompt/` (3), `memory/` (1) | `RegisterPromptSection` |
| Compactor | 1 | `command/` | `RegisterCompactor` |
| Interceptors | varies | any extension | `RegisterInterceptor` |
| Renderers | 0 built-in | any extension | `RegisterRenderer` |
| Providers | 0 built-in | any extension | `RegisterProvider` |

External extensions (TypeScript, Python, Go) use identical API via JSON-RPC over stdin/stdout.

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
  external/    JSON-RPC bridge for TypeScript/Python/Go extensions
tool/          7 built-in tools — extension, not core
command/       18 built-in commands, 5 status sections, 2 shortcuts — extension, not core
prompt/        System prompt builder + prompt section extensions
memory/        Per-project persistent memory — 2 tools, 1 command, 1 prompt section
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
| `PromptSection` | ext | System prompt injection (title, content, order) |

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
- Sessions: `~/.config/piglet/sessions/`
- Extensions: `~/.config/piglet/extensions/`

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
