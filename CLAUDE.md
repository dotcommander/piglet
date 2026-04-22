# Piglet

Extension-first TUI coding assistant. Go 1.26.x · Module: `github.com/dotcommander/piglet`

## Why Piglet Exists

Piglet is **orchestration, not features.** It is a minimal agent engine — provider-agnostic, extension-first — where every capability beyond "stream LLM, execute tools, repeat" lives in an extension.

Existing AI coding assistants (Claude Code, Cursor, etc.) are closed, single-provider, and non-extensible at the architecture level. They are vendor tools for vendor models. Piglet inverts that:

- **Pure orchestration** — The core is an agent loop, streaming, and types. ~1,300 lines. It does not know about files, git, memory, or code. Extensions provide all of that.
- **Provider-agnostic** — Switch between Anthropic, OpenAI, Google, xAI, Groq, OpenRouter, or local models mid-session. No vendor lock.
- **Extension-first** — Built-in tools use the same `ext.App` API as external extensions. Nothing is privileged. Any behavior can be replaced, intercepted, or augmented.
- **Terminal-resident** — Lives where the work happens. No browser, no IDE plugin dependency. Sessions persist across days.
- **User-owned** — All prompts, behavior, skills, and memory live in `~/.config/piglet/` as plain files. Go code reads config; it never contains content.

The core is frozen at 17 events and 5 primitives. The answer to "how do I add X?" is always "write an extension." Piglet must remain small. Features are extensions. If it can't be an extension, question whether it belongs.

## Architecture: Extension-First (Extension-Only If We Could)

Piglet's core is deliberately minimal — an agent loop, streaming, and types. **Everything else is an extension.** The binary ships with a small set of compiled-in extensions (`tool/`, `command/`, `prompt/`). Additional extensions run as standalone binaries via JSON-RPC over stdin/stdout, built from source in `extensions/` and installed to `~/.config/piglet/extensions/`.

**The rule**: New functionality MUST register through `ext.App`. Never wire behavior directly into `core/` or `cmd/piglet/main.go`. The architecture test (`ext/architecture_test.go`) enforces dependency boundaries — violations break the build.

### Dependency Direction (enforced by test)

```
core/       → imports NOTHING from piglet (agent loop, streaming, types)
ext/        → core/ only (registration surface)
tool/, command/, prompt/ → ext/, core/ (extensions — same API as external)
shell/      → ext/, core/, session/ (agent lifecycle — shared between frontends)
tui/, cmd/  → anything (wiring layer)
```

### What Registers Through ext.App

**Compiled-in** (ship with the binary):

| Kind | Count | Source | API |
|------|-------|--------|-----|
| Tools | 4 | `tool/` (read, write, edit, bash) | `RegisterTool` |
| Commands | 3 | `command/` (update, upgrade, mouse) | `RegisterCommand` |
| Shortcuts | 0 | — (session/model shortcuts are in extensions/sessioncmd) | `RegisterShortcut` |
| Status sections | 7 | `command/` | `RegisterStatusSection` |
| Prompt sections | 0 | — (selfknowledge moved to pack-core) | `RegisterPromptSection` |

**External** (consolidated packs — single Go binaries via JSON-RPC, source in `extensions/packs/`):

| Pack | Contains | Registers |
|------|----------|-----------|
| `pack-context` | memory, skill, gitcontext, behavior, projectdocs, prompts, session-tools, inbox, distill, recall, route | 6 tools, 6 commands, 5 prompt sections, 1 compactor, 3 event handlers, 1 message hook |
| `pack-code` | lsp, repomap, sift, plan, suggest, filetools, toolsearch, fossil | 10 tools, 1 command, 4 prompt sections, 2 interceptors, 2 event handlers |
| `pack-agent` | safeguard, rtk, autotitle, clipboard, subagent, provider, loop | 2 tools, 3 commands, 2 prompt sections, 2 interceptors, 1 shortcut, 1 event handler, stream providers |
| `pack-core` | admin, export, extensions-list, undo, scaffold, background, sessioncmd, cmdcore (help, clear, step, compact, quit), selfknowledge | 13 commands, 2 shortcuts, 1 prompt section |
| `pack-workflow` | pipeline, bulk, webfetch, cache, usage, modelsdev | 7 tools, 3 commands, 3 prompt sections, 1 event handler |
| `pack-cron` | cron | 4 tools, 1 command (8 subcommands), 1 event handler |
| `mcp` | mcp | dynamic tools, 1 command, 1 prompt section |

All extensions (compiled-in and external) use the same `ext.App` API. External extensions communicate via JSON-RPC v2 over FD 3/4 (with stdin/stdout fallback) using the Go SDK (`sdk/`).

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
| `command/` | React + Observe | update, upgrade, mouse commands + status sections + prompt-budget handler | compiled-in |
| `pack-context` | Inject + React + Hook + Observe | Memory, skills, git context, behavior, prompts, session tools, sessioncmd (model/session/tree commands + shortcuts) | external pack |
| `pack-code` | Inject + Intercept + React + Observe | LSP, repo map, sift, plan, suggest | external pack |
| `pack-agent` | Inject + Intercept + React + Observe | Safeguard, RTK, autotitle, clipboard, subagent, provider, loop | external pack |
| `pack-core` | Inject + React | help, clear, step, compact, quit commands + selfknowledge prompt section | external pack |
| `pack-workflow` | Inject + React + Observe | Pipeline, bulk, webfetch, cache, usage, modelsdev | external pack |
| `mcp` | Inject + React | Prompt section + dynamic tools bridged from MCP servers | external |

**New features should use existing primitives, not add new ones.**

For guidance on when to use native extensions vs MCP servers, see [docs/extensions-vs-mcp.md](docs/extensions-vs-mcp.md).

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

External extensions use the Go SDK (`sdk/`):

```go
// Example: extensions/safeguard/cmd/main.go
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
cmd/*/         Standalone CLI tools (repomap, pipeline, bulk, confirm, etc.)
core/          Agent loop, streaming, types. Imports nothing from piglet.
ext/           Registration surface (ext.App) — the central API
  app.go       Struct, NewApp, Bind, action queue
  registry.go  Register* methods, interceptor chain
  queries.go   Getter methods (Tools, Commands, etc.)
  runtime.go   Agent facade (SendMessage, Provider, etc.)
  domain.go    Session/model domain methods
  events.go    Event handler dispatch (Observe primitive)
  external/    JSON-RPC v2 bridge for external extensions (Go/TypeScript/Python)
extensions/    External extension source (packs, standalone extensions)
  packs/       Pack bundles (core, agent, context, code, workflow, cron, eval)
  */           Individual extension packages (memory, safeguard, lsp, etc.)
sdk/           Go Extension SDK (import as github.com/dotcommander/piglet/sdk)
tool/          Compiled-in tools (see tool/register.go)
command/       Compiled-in commands, status sections, shortcuts (see command/builtins.go)
prompt/        System prompt builder + compiled-in prompt sections
config/        Settings (YAML), auth (JSON)
provider/      OpenAI-compatible streaming (OpenRouter, xAI, Groq, LM Studio, Ollama). Anthropic/Google via extensions.
session/       Tree-structured JSONL persistence, in-place branching, compaction (see docs/sessions.md)
shell/         Agent lifecycle — submit, process events, drain actions (frontend-agnostic)
tui/           Bubble Tea v2 UI (consumes shell/)
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
| `Shell` | shell | Agent lifecycle manager — submit, events, notifications |

## Build & Test

```bash
go build ./...
go test -race ./... | tail -50
go vet ./...
```

## Binary

```bash
go build -o piglet ./cmd/piglet/
```

## Extensions

Extension source lives in `extensions/` within this repo. Extensions are consolidated into packs — each pack is a single Go binary that bundles multiple related extensions. Packs are built and installed to `~/.config/piglet/extensions/`.

```bash
/extensions install          # Builds all packs from repo, installs to ~/.config/piglet/extensions/
/extensions update           # Rebuild packs from latest source
make extensions              # Build packs directly from local source
```

Without extensions, piglet starts as a minimal agent with only compiled-in tools and commands. With extensions installed, full functionality is available (interceptors, shortcuts, event handlers, message hooks, additional tools/commands — plus dynamic MCP tools). Run `/extensions` for the live inventory.

## Development Workflow

One repo, one module, one tag, one deploy.

### Day-to-day development

```bash
go build ./...                 # Build everything (core + extensions)
go test -race ./...            # Run all tests
make extensions                # Build packs to ~/.config/piglet/extensions/
```

### Deploying to GitHub

```bash
just deploy                    # Preflight, build, test, tag, push, release
just deploy-dry                # Preview deployment plan without executing
```

The deploy recipe: checks working tree is clean, bumps patch version, runs full build+test, tags, pushes, creates GitHub release.

**GitHub Release is mandatory**: `piglet update` self-update uses the GitHub Releases API (`/releases/latest`), NOT git tags. A tag without a release is invisible to self-update.

### Update caching

`piglet update` (remote mode) caches the last successful build's commit hash in `~/.config/piglet/extensions/.last-build-hash`. If the remote HEAD matches, it prints "Extensions already up to date" and skips the clone+build cycle. Local mode (`--local`) always rebuilds.

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
| bubbles | `charm.land/bubbles/v2` v2.1.0 |
| lipgloss | `charm.land/lipgloss/v2` v2.0.2 |
| glamour | `github.com/charmbracelet/glamour` v1.0.0 |
| go-rod | `github.com/go-rod/rod` v0.116.2 |
| colly | `github.com/gocolly/colly/v2` v2.3.0 |
| mcp-go | `github.com/mark3labs/mcp-go` v0.46.0 |

## Core Freeze (BLOCKING)

`core/` is **frozen**. No new types, events, methods, or behavior changes. All future functionality lives in extensions. The 17 agent events and 5 extension primitives are the complete API surface.

### Agent Events (complete — do not add more)

| Event | When | Payload |
|-------|------|---------|
| `EventAgentStart` | Agent loop begins | — |
| `EventAgentEnd` | Agent loop finished | `Messages` |
| `EventSessionLoad` | Pre-loaded messages exist at start | `MessageCount` |
| `EventAgentInit` | Agent configured, before first message | `ToolCount` |
| `EventPromptBuild` | Final system prompt assembled | `System` |
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

### What "frozen" means

- **No new events** — 17 events cover the full lifecycle. Extensions observe, they don't need new hooks.
- **No new types** — Message/Content unions are sealed. Encode custom data in `TextContent` or `ToolResult.Details`.
- **No new methods** — Agent API is complete. Extensions interact through `ext.App`, not `core.Agent`.
- **Bug fixes only** — Security patches and correctness fixes are allowed. Feature additions are not.
- **Test gate** — `ext/architecture_test.go` enforces dependency direction. Core drift breaks the build.

## Conventions

- No `init()`, no mutable package globals
- Short functions (80 lines max)
- Pointer receivers by default
- `context.Context` as first param
- `fmt.Errorf` with `%w` for error wrapping
- **New functionality = new extension, never core modification**

## Release Safety (BLOCKING — public repo)

### Never Commit

| Category | Examples |
|----------|---------|
| API keys / secrets | `.env`, `auth.json`, tokens, passwords, key material |
| User config | `~/.config/piglet/config.yaml`, `models.yaml`, session JSONL |
| Local paths | `/Users/<name>/`, `~/www/`, absolute paths to user directories |
| Scratch / work | `.work/`, `/tmp/` test scripts, one-off debug files |
| Binary artifacts | The `piglet` binary, `.so`, `.dylib` |
| Prompt content | `prompt.md`, `behavior.md` — these are user config, not source |

### Pre-Commit Gate

Before EVERY commit:

1. **`git diff --cached`** — read the full staged diff. Look for hardcoded paths, API keys, user-specific config, or debug print statements.
2. **`git diff --cached --name-only`** — verify no unexpected files. Especially: no binaries, no `~/.config/` content, no `.work/` artifacts.
3. **No absolute user paths** — grep the diff for `/Users/`, `/home/`, `~/`. Config code may reference `os.UserConfigDir()` (fine) but never literal home paths.
4. **No embedded secrets** — grep for `sk-`, `api_key`, `bearer`, `password`. The `config/auth.go` reads keys at runtime — keys never appear in source.
5. **No test scripts in repo** — `/tmp/` test files stay in `/tmp/`.

### Pre-Tag Gate (before `git tag`)

1. **All tests pass**: `go test -race ./... | tail -30`
2. **Build clean**: `go build ./... 2>&1 | tail -10`
3. **go.mod current**: `go mod tidy && go build ./...` — stale deps break `go install` consumers
4. **Smoke test**: `go build -o piglet ./cmd/piglet/ && ./piglet --version`
5. **Architecture test**: `go test ./ext/... -run TestArchitecture` — dependency boundaries enforced
6. **No WIP commits**: `git log v<prev>..HEAD --oneline` — every commit should be shippable
7. **Extensions build**: `for p in core agent context code workflow cron eval; do go build -o /dev/null ./extensions/packs/$p/; done` — compile check only; `-o /dev/null` prevents stray binaries in repo root
8. **Extension list current**: `defaultOfficialExtensions` in `config/config.go` must list all packs (`pack-core`, `pack-agent`, `pack-context`, `pack-code`, `pack-workflow`, `pack-cron`) plus standalone extensions (`mcp`). Verify pack contents match `extensions/packs/*/main.go` imports.

### Pre-Push Gate

1. **Review commit list**: `git log origin/main..HEAD --oneline` — every commit is about to be permanent
2. **No force push to main** — ever
3. **No amended published commits** — create new commits to fix mistakes

### .gitignore Hygiene

These MUST be in `.gitignore` and stay there:

```
/piglet              # binary
.work/               # scratch/specs/audits
.DS_Store            # macOS
repomix-output.md    # export artifacts
```

Periodically verify: `git ls-files --others --ignored --exclude-standard | head -20` — nothing sensitive should appear.

## Violation Log

| Rule | Violations | Last |
|------|-----------|------|
| Extension list current: `defaultOfficialExtensions` had 12 of 31 extensions, stale binaries persisted after update (resolved: consolidated to 5 packs + 1 standalone) | 1 | 2026-03-26 |
