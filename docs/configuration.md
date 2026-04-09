# Configuration

- [Config File Location](#config-file-location)
- [File Overview](#file-overview)
- [General Settings](#general-settings)
- [Agent Settings](#agent-settings)
- [Tool Settings](#tool-settings)
- [Bash Settings](#bash-settings)
- [Git Settings](#git-settings)
- [Shortcuts](#shortcuts)
- [Prompt Order](#prompt-order)
- [Project Docs](#project-docs)
- [Local Servers](#local-servers)
- [Extension Settings](#extension-settings)
- [Sub-Agent Settings](#sub-agent-settings)
- [Debug Mode](#debug-mode)
- [Environment Variables](#environment-variables)
- [Full Example](#full-example)

## Config File Location

All configuration lives under `~/.config/piglet/` (respects `XDG_CONFIG_HOME`):

```
~/.config/piglet/
├── config.yaml          # Main settings
├── auth.json            # API keys
├── models.yaml          # Model catalog
├── prompt.md            # System prompt (identity)
├── behavior.md          # Behavioral guidelines
├── history              # Input history
├── skills/              # Markdown skill files
├── sessions/            # Conversation history (JSONL)
└── extensions/          # Extension binaries and configs
```

## File Overview

| File | Format | Purpose |
|------|--------|---------|
| `config.yaml` | YAML | All settings documented on this page |
| `auth.json` | JSON | API keys per provider (see [Providers](providers.md)) |
| `models.yaml` | YAML | Model catalog — names, costs, context windows |
| `prompt.md` | Markdown | System prompt identity text; overrides `systemPrompt` in config |
| `behavior.md` | Markdown | Behavioral guidelines injected into the prompt |

Piglet creates `config.yaml` and `models.yaml` automatically on first run. The other files are optional.

## General Settings

```yaml
defaultModel: claude-opus-4-6      # Model ID used by default
defaultProvider: anthropic          # Provider name (usually inferred from model)
smallModel: claude-haiku-4-5       # Model for lightweight tasks (auto-title, compaction)
systemPrompt: "You are piglet..."  # Base identity (prompt.md overrides this)
theme: ""                          # Reserved for future use
rtk: null                          # RTK token optimization: null=auto, true/false=explicit
debug: false                       # Log all request/response payloads
safeguard: null                    # Dangerous command blocking: null/true=enabled, false=disabled
deferredToolsNote: ""              # Custom instruction for the model about deferred tools (local models only)
```

### Model Resolution

The default model is resolved in this order:

1. `--model` flag (highest priority)
2. `PIGLET_DEFAULT_MODEL` environment variable
3. `defaultModel` in `config.yaml`

The small model follows a similar cascade:

1. `PIGLET_SMALL_MODEL` environment variable
2. `smallModel` in `config.yaml`
3. Falls back to the default model

## Agent Settings

Control how the agent loop behaves:

```yaml
agent:
  maxTurns: 10             # Max LLM calls per prompt (0 = unlimited)
  bgMaxTurns: 5            # Max turns for background agent
  autoTitle: true          # Auto-generate session titles after first exchange
  compactKeepRecent: 6     # Messages to preserve during compaction
  compactAt: 0             # Token threshold for auto-compaction (0 = disabled)
  maxMessages: 0           # Hard cap on conversation length (0 = unlimited)
  maxTokens: 0             # Output token limit (0 = model default)
  maxRetries: 3            # Retry attempts on transient errors
  toolConcurrency: 10      # Max parallel tool executions
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `maxTurns` | int | `10` | Maximum LLM calls per user prompt. Each tool-use round counts as one turn. Set to `0` for unlimited |
| `bgMaxTurns` | int | `5` | Maximum turns for the background agent (`/bg`) |
| `autoTitle` | bool | `true` | Automatically generate a session title after the first exchange |
| `compactAt` | int | `0` | Token threshold for auto-compaction. When input tokens exceed this value, piglet compacts the history. `0` disables auto-compaction |
| `compactKeepRecent` | int | `6` | Number of recent messages to preserve when compacting |
| `maxMessages` | int | `0` | Hard cap on conversation length. When exceeded, oldest messages (except the first) are dropped. `0` = unlimited |
| `maxTokens` | int | `0` | Output token limit sent to the provider. `0` uses the model's default |
| `maxRetries` | int | `3` | Number of retry attempts on transient provider errors (rate limits, timeouts) |
| `toolConcurrency` | int | `10` | Maximum number of tool calls executed in parallel |

### Auto-Compaction

When `compactAt` is set, piglet automatically compacts the conversation when input tokens exceed the threshold. This keeps long sessions from hitting context limits. The `compactKeepRecent` most recent messages are always preserved.

```yaml
agent:
  compactAt: 100000        # Compact when input tokens exceed 100k
  compactKeepRecent: 8     # Keep 8 most recent messages
```

If an extension registers a compactor (e.g., the context pack's LLM-based compactor), it produces an intelligent summary. Otherwise, piglet falls back to a static summary that preserves the first message plus the most recent ones.

## Tool Settings

Configure built-in tool behavior:

```yaml
tools:
  readLimit: 2000          # Max lines per file read (default: 2000)
  grepLimit: 100           # Max grep matches returned (default: 100)
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `readLimit` | int | `2000` | Maximum number of lines returned by the `read` tool per invocation |
| `grepLimit` | int | `100` | Maximum number of matches returned by the `grep` tool |

## Bash Settings

Control the shell tool's limits:

```yaml
bash:
  defaultTimeout: 30       # Default command timeout in seconds
  maxTimeout: 300           # Maximum allowed timeout in seconds
  maxStdout: 100000         # Max stdout capture in bytes
  maxStderr: 50000          # Max stderr capture in bytes
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `defaultTimeout` | int | `30` | Default timeout for shell commands in seconds |
| `maxTimeout` | int | `300` | Maximum timeout the model can request (hard cap at 5 minutes) |
| `maxStdout` | int | `100000` | Maximum bytes captured from stdout (~100KB) |
| `maxStderr` | int | `50000` | Maximum bytes captured from stderr (~50KB) |

## Git Settings

Control git context injected into the system prompt (requires the git context extension):

```yaml
git:
  maxDiffStatFiles: 30     # Max files shown in diff stat
  maxLogLines: 5           # Recent commits shown
  maxDiffHunkLines: 50     # Max lines per diff hunk
  commandTimeout: 5        # Git command timeout in seconds
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `maxDiffStatFiles` | int | `30` | Maximum files shown in the diff stat summary |
| `maxLogLines` | int | `5` | Number of recent commits included in the prompt |
| `maxDiffHunkLines` | int | `50` | Maximum lines per diff hunk |
| `commandTimeout` | int | `5` | Timeout for individual git commands in seconds |

## Shortcuts

Override the default keyboard shortcuts:

```yaml
shortcuts:
  model: ctrl+p            # Open model selector (default: ctrl+p)
  session: ctrl+s          # Open session picker (default: ctrl+s)
```

Values use the format recognized by Bubble Tea: `ctrl+x`, `alt+x`, `shift+tab`, etc.

| Action | Default | Description |
|--------|---------|-------------|
| `model` | `ctrl+p` | Open the model selector |
| `session` | `ctrl+s` | Open the session picker |

Extensions may register additional shortcuts. Use `/help` to see all active shortcuts.

## Prompt Order

Override the display order of system prompt sections. Lower numbers appear earlier:

```yaml
promptOrder:
  "Self-Knowledge": 10
  "Project Instructions": 20
  "Git Context": 30
  "Memory": 50
  "Skills": 60
  "Behavior": 70
```

This is useful when extensions inject prompt sections and you want to control their relative position.

## Project Docs

Auto-read files from the working directory and inject them as prompt sections:

```yaml
projectDocs:
  - name: CLAUDE.md
    title: "Project Instructions"
  - name: agents.md
    title: "Agents"
```

By default, piglet reads `CLAUDE.md` and `agents.md` if they exist in the current directory. Add entries to include additional files. Each file's content becomes a prompt section with the given title.

## Local Servers

Probe local model servers on startup and register their models:

```yaml
localServers:
  - http://localhost:1234     # LM Studio
  - http://localhost:11434    # Ollama

localDefaults:
  contextWindow: 8192         # Default context window for local models
  maxTokens: 4096             # Default max output tokens for local models
```

Piglet probes each URL's `/v1/models` endpoint, auto-detects the server type (LM Studio, Ollama, etc.), and registers discovered models. See [Providers — Local Models](providers.md#local-models) for details.

## Extension Settings

### Disabling Extensions

Skip specific extensions during loading:

```yaml
disabled_extensions:
  - pack-workflow
  - pack-cron
```

Extensions are disabled by name. Use the `/extensions` command to see loaded extension names. Changes take effect on the next launch. For a full guide, see [Extensions — Disabling Extensions](extensions.md#disabling-extensions).

### Project-Local Extensions

Allow extensions from the current project's `.piglet/extensions/` directory:

```yaml
allowProjectExtensions: true   # Default: false
```

This is disabled by default for security — project-local extensions execute code. Only enable this for trusted projects.

### Installation Source

Override the extension repository or limit which packs to install:

```yaml
extInstall:
  repoUrl: https://github.com/dotcommander/piglet-extensions.git
  official:
    - pack-core
    - pack-agent
    - pack-context
    - pack-code
    - pack-workflow
    - pack-cron
    - mcp
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `repoUrl` | string | `https://github.com/dotcommander/piglet-extensions.git` | Git repository for official extensions |
| `official` | list | All packs + mcp | Extension pack names to install |

### Provider Base URL Overrides

Point a provider at a custom endpoint (proxy, gateway, self-hosted):

```yaml
providers:
  openai: https://my-proxy.example.com/v1
  anthropic: https://anthropic-proxy.internal
```

## Sub-Agent Settings

Control sub-agents dispatched by the `dispatch` tool:

```yaml
subagent:
  maxTurns: 10             # Max turns per sub-agent (default: 10)
```

## Debug Mode

Enable full payload logging:

```yaml
debug: true
```

When enabled, piglet writes all LLM request and response payloads to `~/.config/piglet/debug.log`. Useful for diagnosing provider issues or inspecting prompt construction.

You can also enable debug mode per-session with the `--debug` flag:

```bash
piglet --debug
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `PIGLET_DEFAULT_MODEL` | Override default model (takes precedence over config) |
| `PIGLET_SMALL_MODEL` | Override small model for background tasks |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `GOOGLE_API_KEY` | Google API key |
| `GEMINI_API_KEY` | Google API key (alias) |
| `XAI_API_KEY` | xAI (Grok) API key |
| `GROQ_API_KEY` | Groq API key |
| `OPENROUTER_API_KEY` | OpenRouter API key |
| `ZAI_API_KEY` | Z.AI API key |
| `XDG_CONFIG_HOME` | Override config directory base (default: `~/.config`) |

## Full Example

A complete `config.yaml` showing all available settings:

```yaml
defaultModel: claude-opus-4-6
smallModel: claude-haiku-4-5
debug: false
safeguard: true
rtk: null
deferredToolsNote: ""              # Custom instruction for deferred tools (local models only)

agent:
  maxTurns: 15
  bgMaxTurns: 5
  autoTitle: true
  compactAt: 120000
  compactKeepRecent: 8
  maxMessages: 0
  maxTokens: 0
  maxRetries: 3
  toolConcurrency: 10

tools:
  readLimit: 2000
  grepLimit: 100

bash:
  defaultTimeout: 30
  maxTimeout: 300
  maxStdout: 100000
  maxStderr: 50000

git:
  maxDiffStatFiles: 30
  maxLogLines: 5
  maxDiffHunkLines: 50
  commandTimeout: 5

shortcuts:
  model: ctrl+p
  session: ctrl+s

promptOrder:
  "Self-Knowledge": 10
  "Project Instructions": 20
  "Git Context": 30

projectDocs:
  - name: CLAUDE.md
    title: "Project Instructions"
  - name: agents.md
    title: "Agents"

localServers:
  - http://localhost:1234

localDefaults:
  contextWindow: 8192
  maxTokens: 4096

disabled_extensions:
  - pack-cron

allowProjectExtensions: false

subagent:
  maxTurns: 10

providers:
  openai: https://my-proxy.example.com/v1

extInstall:
  repoUrl: https://github.com/dotcommander/piglet-extensions.git
  official:
    - pack-core
    - pack-agent
    - pack-context
    - pack-code
    - pack-workflow
    - pack-cron
    - mcp
```
