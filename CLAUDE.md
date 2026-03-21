# Piglet

Minimalist TUI coding assistant. Go 1.26.x · Module: `github.com/dotcommander/piglet`

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

## Layout

```
cmd/piglet/           Entry point, TUI vs print mode
core/                 Agent loop, types, events, provider interface
config/               Settings (YAML), auth (JSON)
provider/             OpenAI, Anthropic, Google streaming providers
session/              JSONL conversation persistence, compaction
tool/                 Built-in tools (read, write, edit, bash, grep, find, ls)
command/              Built-in slash commands (registered via ext API)
tui/                  Bubble Tea v2 UI (app, input, modal, status, message, theme)
ext/                  Extension API (tools, commands, shortcuts, interceptors, prompt sections)
prompt/               System prompt builder (user file + extensions + tool hints)
examples/extensions/  Example extensions
```

## Key Types

| Type | Package | Purpose |
|------|---------|---------|
| `Agent` | core | Agent loop with streaming, tools, steering |
| `Model` | core | LLM endpoint definition |
| `StreamProvider` | core | Interface all providers implement |
| `Registry` | provider | Model catalog + provider factory |
| `Session` | session | JSONL conversation persistence |
| `App` | ext | Extension registration surface |
| `Model` (TUI) | tui | Bubble Tea v2 app model |

## Dependencies

| Package | Version |
|---------|---------|
| bubbletea | `charm.land/bubbletea/v2` v2.0.2 |
| bubbles | `charm.land/bubbles/v2` v2.0.0 |
| lipgloss | `charm.land/lipgloss/v2` v2.0.0 |
| glamour | `github.com/charmbracelet/glamour` v1.0.0 |

## Config

- Settings: `~/.config/piglet/config.yaml`
- Auth: `~/.config/piglet/auth.json`
- System prompt: `~/.config/piglet/prompt.md` (overrides default identity)
- Sessions: `~/.config/piglet/sessions/`

## Extension Points

All built-in functionality registers through the ext API:

| What | Registration | Package |
|------|-------------|---------|
| Tools (7) | `ext.RegisterTool` | `tool/` |
| Commands (8) | `ext.RegisterCommand` | `command/` |
| Shortcuts (Ctrl+P, Ctrl+S) | `ext.RegisterShortcut` | `command/` |
| Prompt sections | `ext.RegisterPromptSection` | any extension |
| Interceptors | `ext.RegisterInterceptor` | any extension |
| Renderers | `ext.RegisterRenderer` | any extension |
| Providers | `ext.RegisterProvider` | any extension |

## Conventions

- No `init()`, no mutable package globals
- Short functions (80 lines max)
- Pointer receivers by default
- `context.Context` as first param
- `fmt.Errorf` with `%w` for error wrapping
