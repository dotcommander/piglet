<p align="center">
  <img src="docs/piglet.png" alt="piglet" width="300">
</p>

<h1 align="center">piglet</h1>

<p align="center">
  A coding assistant that lives in your terminal.<br>
  Multi-provider, extension-first, built in Go.
</p>

<p align="center">
  <a href="#install">Install</a> · <a href="#extensions">Extensions</a> · <a href="#models">Models</a> · <a href="docs/getting-started.md">Getting Started</a> · <a href="docs/building-extensions.md">Write Your Own</a>
</p>

---

## What It Does

Piglet is a terminal-native coding assistant. It reads your files, runs commands, edits code in place, and persists sessions across days. No copy-pasting, no browser, no IDE plugin.

```
$ piglet
> refactor the auth middleware to use errgroup

  Reading cmd/server/middleware.go...
  Editing cmd/server/middleware.go...
  Running go build ./...
  Running go test ./cmd/server/...

  Done. Replaced sequential error handling with errgroup.
  3 files changed, tests passing.
```

One-shot mode for quick questions:

```bash
piglet "what does this function do"
piglet "find everywhere we call the payments API"
```

## Install

Requires Go 1.26+.

```bash
go install github.com/dotcommander/piglet/cmd/piglet@latest
```

Verify: `piglet --version`

Set an API key and run it. First launch auto-detects your keys, picks a default model, and writes config to `~/.config/piglet/`.

```bash
export ANTHROPIC_API_KEY=sk-ant-...   # or OPENAI_API_KEY, GOOGLE_API_KEY, etc.
piglet
```

> If no API key is set, piglet tells you exactly which environment variables to set and still writes its config files so you're ready once you add a key.

### Extensions

First launch automatically builds and installs the [official extensions](https://github.com/dotcommander/piglet-extensions) (memory, skills, LSP, safeguard, and more). You'll see per-extension progress on stderr — it takes a minute or two, then you're fully loaded.

To rebuild extensions later (e.g. after an update), run `/update` inside piglet.

### Build from Source

```bash
git clone https://github.com/dotcommander/piglet
cd piglet
go build -o piglet ./cmd/piglet/
```

## First Things to Try

```
piglet                          # interactive mode — ask anything
piglet "explain this repo"      # one-shot from any project directory
```

Inside a session:

| Try this | What happens |
|----------|-------------|
| Ask a question about your code | Piglet reads the relevant files and answers |
| `fix the failing test` | Reads test output, edits code, runs tests again |
| `/help` | See all available commands |
| `/model` | Switch to a different LLM mid-session |
| `Ctrl+C` | Stop streaming or exit |

## How It Works

Piglet is an agent loop: you send a message, the LLM responds with text or tool calls, piglet executes the tools, feeds results back, and repeats until the LLM is done. Streaming output appears as it's generated.

**Built-in tools**: `read`, `write`, `edit`, `bash`, `grep`, `find`, `ls` — everything the LLM needs to navigate and modify a codebase.

**Extensions** add everything else: project memory that persists across sessions, on-demand skill loading, LSP-powered code intelligence, repository structure maps, dangerous command blocking, clipboard image pasting, sub-agent delegation, and structured task planning. Extensions are standalone binaries that communicate via JSON-RPC — write them in Go, TypeScript, or Python.

### Architecture

```
core/       Agent loop, streaming, types. Imports nothing from piglet.
ext/        Registration surface (ext.App) — the central API.
tool/       7 built-in tools (read, write, edit, bash, grep, find, ls).
command/    16 slash commands, 1 keyboard shortcut, 6 status sections.
prompt/     System prompt builder + 2 prompt sections.
provider/   OpenAI, Anthropic, Google streaming + local server auto-discovery.
shell/      Agent lifecycle — submit, events, notifications (frontend-agnostic).
tui/        Bubble Tea v2 terminal UI (consumes shell/).
sdk/        Go Extension SDK — standalone module (github.com/dotcommander/piglet/sdk).
```

Everything is an extension. The core agent loop knows nothing about files, git, or code — tools and extensions provide all capabilities through a single registration API (`ext.App`). The [architecture test](ext/architecture_test.go) enforces dependency boundaries at build time.

## Models

Switch mid-session with `Ctrl+P` or `/model`. No restart needed.

| Provider | Env Variable |
|----------|--------------|
| Anthropic | `ANTHROPIC_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |
| Google | `GOOGLE_API_KEY` |
| xAI | `XAI_API_KEY` |
| Groq | `GROQ_API_KEY` |
| OpenRouter | `OPENROUTER_API_KEY` |
| Z.AI | `ZAI_API_KEY` |
| LM Studio | — (localhost:1234) |
| Ollama | — (localhost:11434) |

Any OpenAI-compatible endpoint works via base URL override in config.

Use just the model ID (`gpt-5`) or the full form (`openai/gpt-5`). Override with `PIGLET_DEFAULT_MODEL` or `defaultModel` in config. Run `/model` to see all available models.

> Piglet writes `models.yaml` to `~/.config/piglet/` on first run with the current model catalog. To refresh after a piglet upgrade, delete the file and restart — it regenerates automatically.

### Local Models

No API key needed. Point piglet at any local server that speaks the OpenAI protocol:

```bash
piglet --model :1234          # LM Studio (default port)
piglet --model :11434         # Ollama (default port)
piglet --model :8080          # llama.cpp, MLX, vLLM
```

Piglet probes the server, discovers available models, and auto-detects the server type. For persistent auto-discovery on every startup:

```yaml
# ~/.config/piglet/config.yaml
localServers:
  - http://localhost:1234
  - http://localhost:11434
```

Local models automatically get **progressive tool disclosure** — piglet sends only 7 core tool schemas and defers the remaining 42 tools behind a lightweight `tool_search` index. The model calls `tool_search` when it needs a tool, and the full schema is loaded on demand. This keeps the prompt small enough for models with limited context windows.

See [Providers — Local Models](docs/providers.md#local-models) for the full guide.

## Commands and Shortcuts

| Command | Action |
|---------|--------|
| `/help` | List available commands |
| `/model` | Switch model |
| `/session` | List and switch sessions |
| `/branch` | Fork current session |
| `/clear` | Clear conversation |
| `/compact` | Summarize to free context |
| `/search` | Search session history |
| `/export` | Export current session |
| `/undo` | Undo last file change |
| `/step` | Toggle step-by-step tool approval |
| `/bg` | Run a prompt in a background agent |
| `/config` | Show or set up configuration |
| `/extensions` | List loaded extensions |
| `/quit` | Exit |

| Key | Action |
|-----|--------|
| `Ctrl+P` | Model selector |
| `Ctrl+S` | Session picker |
| `Ctrl+V` | Paste clipboard image |
| `Ctrl+C` | Stop streaming / quit |
| `Enter` | Send |
| `Alt+Enter` | Newline |

## Extensions

Piglet's extension system is the same API used by its own built-in tools. Extensions register through five primitives:

| Primitive | What it does | Example |
|-----------|-------------|---------|
| **Inject** | Add text to the system prompt | Memory facts, skill instructions |
| **React** | Respond to triggers | Tools the LLM calls, slash commands |
| **Intercept** | Modify or block tool calls | Safeguard blocks `rm -rf /`, RTK rewrites bash for token savings |
| **Hook** | Process user messages before the LLM | Skill trigger matching |
| **Observe** | React to lifecycle events | Auto-title sessions after first exchange |

### Extension Packs

Piglet works without any extensions — the core agent has built-in tools, commands, and providers. Extensions add everything beyond the baseline.

Official extensions ship as consolidated packs (fewer processes, faster startup). Install with [`piglet-extensions`](https://github.com/dotcommander/piglet-extensions):

| Pack | Extensions | Purpose |
|------|-----------|---------|
| **pack-context** | memory, skill, gitcontext, behavior, prompts, session-tools, inbox | Context injection — memory, skills, git status, behavior guidelines |
| **pack-code** | lsp, repomap, sift, plan, suggest | Code intelligence — LSP, repo mapping, planning mode |
| **pack-agent** | safeguard, rtk, autotitle, clipboard, subagent, provider, loop | Agent lifecycle — safety, token optimization, delegation |
| **pack-core** | admin, export, extensions-list, undo, scaffold, background | Convenience commands — export, undo, scaffolding |
| **pack-workflow** | pipeline, bulk, webfetch, cache, usage, modelsdev | Workflow tools — pipelines, bulk ops, web fetch |
| **mcp** | — | Bridge MCP servers as piglet tools (standalone) |

### What You Need

Not all packs are required. Here's what matters:

| Pack | Verdict | What you lose without it |
|------|---------|--------------------------|
| **pack-context** | Essential | No session memory, no skills, no git context in prompt |
| **pack-code** | Important | No repo map, no LSP, no planning mode |
| **pack-agent** | Important | No command safety checks, no token optimization |
| **pack-core** | Optional | No `/undo`, `/export`, `/ext-init` — convenience only |
| **pack-workflow** | Optional | No `/pipe`, `/bulk`, web fetch — niche workflows |
| **mcp** | Optional | No MCP server bridging |

Skip optional packs by setting `disabled_extensions` in `config.yaml`:

```yaml
disabled_extensions:
  - pack-core
  - pack-workflow
```

### Write Your Own

Scaffold a new extension:

```bash
/ext-init my-extension
```

Or build one from scratch in Go, TypeScript, or Python. Extensions communicate via JSON-RPC over stdin/stdout — no linking, no shared memory, any language works.

See [docs/building-extensions.md](docs/building-extensions.md) for the SDK reference and examples.

## Configuration

Everything lives in `~/.config/piglet/`:

| File | Purpose |
|------|---------|
| `config.yaml` | Model, shell, theme, agent limits |
| `auth.json` | API keys — literals, `$ENV_VAR`, or `!shell command` |
| `prompt.md` | System prompt (overrides config) |
| `behavior.md` | Behavioral guidelines |
| `models.yaml` | Model catalog |
| `sessions/` | Conversation history (JSONL) |
| `skills/` | Markdown methodology files |
| `prompts/` | Templates that become slash commands |
| `extensions/` | Extension packs and binaries |

```yaml
# config.yaml
defaultModel: claude-opus-4-6
smallModel: claude-haiku-4-5-20251001   # for background tasks
agent:
  maxTurns: 30
  maxMessages: 200
  toolConcurrency: 10
```

Auth supports environment variable references and shell commands:

```json
{
  "anthropic": "$ANTHROPIC_API_KEY",
  "openai": "sk-...",
  "google": "!op read op://vault/google/key"
}
```

Prompt templates in `prompts/` register as slash commands — `prompts/review.md` becomes `/review`:

```markdown
---
description: Review code for issues
---
Review the following for bugs and style problems:

$@
```

See [docs/configuration.md](docs/configuration.md) for the full settings reference.

## License

MIT
