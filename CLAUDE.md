# Piglet

Extension-first TUI coding assistant. Go 1.26.x · Module: `github.com/dotcommander/piglet`

## Architecture: Extension-First (Extension-Only If We Could)

Piglet's core is deliberately minimal — an agent loop, streaming, and types. **Everything else is an extension.** The binary ships with a small set of compiled-in extensions (`tool/`, `command/`, `prompt/`). Twelve extensions run as standalone binaries via JSON-RPC over stdin/stdout, built from source in [`piglet-extensions`](https://github.com/dotcommander/piglet-extensions) and installed to `~/.config/piglet/extensions/`.

**The rule**: New functionality MUST register through `ext.App`. Never wire behavior directly into `core/` or `cmd/piglet/main.go`. The architecture test (`ext/architecture_test.go`) enforces dependency boundaries — violations break the build.

### Dependency Direction (enforced by test)

```
core/       → imports NOTHING from piglet (agent loop, streaming, types)
ext/        → core/ only (registration surface)
tool/, command/, prompt/ → ext/, core/ (extensions — same API as external)
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

**External** (standalone Go binaries via JSON-RPC, source in [`piglet-extensions`](https://github.com/dotcommander/piglet-extensions)):

| Extension | Registers |
|-----------|-----------|
| `safeguard` | 1 interceptor |
| `rtk` | 1 interceptor, 1 prompt section |
| `autotitle` | 1 event handler |
| `clipboard` | 1 tool, 1 shortcut |
| `skill` | 2 tools, 1 command, 1 prompt section, 1 message hook |
| `memory` | 3 tools, 1 command, 1 prompt section, 1 compactor, 2 event handlers |
| `subagent` | 1 tool |
| `lsp` | 5 tools, 1 prompt section |
| `repomap` | 2 tools, 1 prompt section, 1 event handler |
| `plan` | 3 tools, 1 command, 1 prompt section, 1 interceptor |
| `bulk` | 1 tool, 1 prompt section |
| `mcp` | dynamic tools, 1 command, 1 prompt section |

All extensions (compiled-in and external) use the same `ext.App` API. External extensions communicate via JSON-RPC v2 over stdin/stdout using the Go SDK ([`piglet/sdk`](https://github.com/dotcommander/piglet/sdk)).

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
| `safeguard/` | Intercept | Before hook blocks dangerous commands (profiles: strict/balanced/off) | external |
| `rtk/` | Inject + Intercept | Prompt section + bash rewriter | external |
| `skill/` | Inject + React + Hook | Tools + message hook | external |
| `subagent/` | React | Dispatch tool delegates to sub-agents | external |
| `clipboard/` | React | Tool + shortcut for images | external |
| `autotitle/` | Observe | Event handler for session titles | external |
| `lsp/` | Inject + React | Prompt section + tools (code intelligence) | external |
| `repomap/` | Inject + React + Observe | Prompt section + tools + stale-check event handler | external |
| `plan/` | Inject + Intercept + React | Prompt section + interceptor + tools + command | external |
| `bulk/` | Inject + React | Prompt section + tool (parallel map over files/repos) | external |
| `mcp/` | Inject + React | Prompt section + dynamic tools bridged from MCP servers | external |

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

External extensions (in `piglet-extensions` repo) use the Go SDK ([`piglet/sdk`](https://github.com/dotcommander/piglet/sdk)):

```go
// Example: piglet-extensions/safeguard/cmd/main.go
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
sdk/           Go Extension SDK — standalone module (github.com/dotcommander/piglet/sdk)
tool/          7 compiled-in tools
command/       18 compiled-in commands, 5 status sections, 2 shortcuts
prompt/        System prompt builder + 4 compiled-in prompt sections
config/        Settings (YAML), auth (JSON)
provider/      3 streaming protocols: OpenAI (+ compatible: OpenRouter, xAI, Groq, LM Studio, Ollama), Anthropic, Google
session/       JSONL conversation persistence, compaction
tui/           Bubble Tea v2 UI

# External extensions live in separate repo: dotcommander/piglet-extensions
# Source: ~/go/src/piglet-extensions/
# Build: make extensions (delegates to piglet-extensions Makefile)
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

Extensions live in a separate repo: `dotcommander/piglet-extensions` (`~/go/src/piglet-extensions/`).

```bash
/extensions install          # From inside piglet — clones, builds, installs all
/extensions update           # Rebuild from latest source

# Or manually:
cd ~/go/src/piglet-extensions && make extensions           # Build all
cd ~/go/src/piglet-extensions && make extensions-safeguard  # Build one
```

Without extensions, piglet starts as a minimal agent (7 tools, 18 commands, no interceptors/events). With extensions installed, full functionality is available (24+ tools, 21 commands, interceptors, shortcuts, event handlers, message hooks — plus dynamic MCP tools).

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

## Memory Architecture (DO NOT CHANGE)

The memory extension injects a **compact index** (not full content) as a static prompt section at session start. This is the correct design — do not replace with per-turn retrieval, BM25 scoring, or similar complexity.

**How it works:**
1. `MEMORY.md` is an index file: short key-value pairs and references to per-topic files
2. Injected once at session start via `RegisterPromptSection` (order 50)
3. The LLM reads the index, then uses `memory_get` tool to load specific topic files on demand
4. This is **progressive disclosure**: the index stays small, detail is fetched when relevant

**Why "inject all at session start" is correct:**
- The index IS small — it's designed to fit in the prompt budget (~8000 chars cap)
- Per-turn retrieval (BM25, embeddings) adds latency, complexity, and an LLM dependency for something that doesn't need it
- The LLM already knows how to decide what's relevant from an index — that's what indexes are for
- Topic files are loaded via tool calls only when the LLM determines they're needed

**Automatic context extraction:** The memory extension auto-extracts context facts (`ctx:file:*`, `ctx:edit:*`, `ctx:error:*`, `ctx:cmd:*`) from tool results on every `EventTurnEnd`, capped at 50 most recent. When message count exceeds a threshold, it compacts these into a `ctx:summary` via LLM. User facts are created manually via `memory_set`.

**What NOT to do:**
- Do not add BM25 or embedding-based retrieval — the index + tool pattern already achieves relevance filtering
- Do not add effectiveness tracking (surfaced/followed/ignored counters) — piglet users control their own memory files
- Do not look at engram (toejough/engram) as a model — it solves instruction decay for a system where users have no direct control (Claude Code plugins). Piglet's users own their memory files.

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
