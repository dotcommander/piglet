<p align="center">
  <img src="docs/piglet.png" alt="piglet" width="300">
</p>

<h1 align="center">piglet</h1>

<p align="center">A minimalist TUI coding assistant with multi-provider LLM support and a built-in tool suite.</p>

## Features

- Interactive TUI built with Bubble Tea v2
- Print mode for one-shot queries: `piglet "explain this error"`
- Multi-provider: OpenAI, Anthropic, Google, xAI, Groq, OpenRouter
- 7 core tools (read, write, edit, bash, grep, find, ls) + 7 extension tools (memory, skills, clipboard, subagent, and more)
- Session persistence via JSONL — resume where you left off
- Conversation compaction to manage context window
- Customizable system prompt via config or `~/.config/piglet/prompt.md`
- Extension API: register custom tools, commands, shortcuts, prompt sections, interceptors, and event handlers
- Model switcher without restarting (Ctrl+P)

## Install

```bash
go install github.com/dotcommander/piglet/cmd/piglet@latest
```

Or build from source:

```bash
git clone https://github.com/dotcommander/piglet
cd piglet
go build -o piglet ./cmd/piglet/
make extensions   # Build + install extension binaries
```

## Quick Start

On first launch, piglet auto-detects missing config and runs interactive setup — creates `~/.config/piglet/`, writes a default model catalog, and picks the best model based on which API keys it finds in your environment.

```bash
export ANTHROPIC_API_KEY=sk-ant-...   # or OPENAI_API_KEY, GOOGLE_API_KEY, etc.
piglet                                # first run triggers setup automatically
```

You can also run setup explicitly:

```bash
piglet init
```

**Print mode** — answers and exits:

```bash
piglet "what does this function do"
piglet "refactor main.go to use errgroup"
```

## Configuration

Config lives in `~/.config/piglet/`:

| File | Purpose |
|------|---------|
| `config.yaml` | Settings (model, theme, shell, system prompt, extensions) |
| `auth.json` | API keys (stored or referenced) |
| `prompt.md` | Custom system prompt (overrides default identity) |
| `behavior.md` | Behavioral guidelines for the LLM |
| `sessions/` | Persisted conversation history |
| `skills/` | Markdown methodology files |
| `extensions/` | External extension directories |

**config.yaml example:**

```yaml
defaultProvider: anthropic
defaultModel: claude-sonnet-4-20250514
systemPrompt: "You are a senior Go developer. Always write tests."
theme: dark
shellPath: /bin/zsh
extensions:
  - ~/.config/piglet/extensions/my-tools.so
```

**Custom system prompt** — create `~/.config/piglet/prompt.md` for a full custom prompt:

```markdown
You are piglet, a Go specialist. Follow these rules:
- Always use table-driven tests
- Prefer errgroup over sync.WaitGroup
- Use slog for logging
```

The prompt file overrides `systemPrompt` in config.yaml. Extensions can add additional prompt sections.

**Auth setup** — piglet reads API keys from environment variables automatically. You can also store them in `~/.config/piglet/auth.json`:

```json
{
  "openai": "$OPENAI_API_KEY",
  "anthropic": "sk-ant-...",
  "google": "!op read op://vault/google/key"
}
```

Values can be literal keys, `$ENV_VAR` references, or `!shell commands`.

**Environment variables:**

| Variable | Effect |
|----------|--------|
| `PIGLET_DEFAULT_MODEL` | Override model (e.g. `gpt-4.1-mini`) |
| `OPENAI_API_KEY` | OpenAI key (auto-detected) |
| `ANTHROPIC_API_KEY` | Anthropic key (auto-detected) |
| `GOOGLE_API_KEY` | Google key (auto-detected) |

## Providers

| Provider | Example Models | Key Env Var |
|----------|---------------|-------------|
| OpenAI | `gpt-4.1`, `gpt-4.1-mini`, `gpt-4o`, `o3` | `OPENAI_API_KEY` |
| Anthropic | `claude-sonnet-4-20250514`, `claude-haiku-4-5-20251001` | `ANTHROPIC_API_KEY` |
| Google | `gemini-2.5-pro`, `gemini-2.5-flash` | `GOOGLE_API_KEY` |
| xAI | `grok-3` | `XAI_API_KEY` |
| Groq | `llama-3.3-70b-versatile` | `GROQ_API_KEY` |
| OpenRouter | `auto` (routes best available) | `OPENROUTER_API_KEY` |
| LM Studio | `local-model` (localhost:1234) | — |

Specify a model with `PIGLET_DEFAULT_MODEL` or set `defaultModel` in config. You can use just the model ID (`gpt-4.1`) or the full `provider/model-id` form (`openai/gpt-4.1`).

## Commands

**Slash commands** (type in the input box):

| Command | Action |
|---------|--------|
| `/help` | Show available commands |
| `/clear` | Clear conversation history |
| `/step` | Toggle step-by-step mode |
| `/model` | Switch model |
| `/modelsync` | Refresh available models |
| `/session` | List and load sessions |
| `/branch` | Fork current session |
| `/title` | Set session title |
| `/compact` | Summarize conversation to free context |
| `/search` | Search conversation history |
| `/export` | Export current session |
| `/undo` | Undo last file change |
| `/config` | Show or set up configuration |
| `/extensions` | List loaded extensions |
| `/bg` | Run a background agent |
| `/quit` | Exit |

All built-in commands register through the extension API and can be overridden.

**Keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `Ctrl+C` | Stop streaming / quit |
| `Ctrl+P` | Open model selector |
| `Ctrl+S` | Open session picker |
| `Ctrl+V` | Paste clipboard image |
| `Enter` | Send message |
| `Alt+Enter` | Insert newline |

Shortcuts also register through the extension API and can be customized.

## Extensions

Piglet's architecture is extension-first. Seven built-in extensions (safeguard, rtk, autotitle, clipboard, skill, memory, subagent) run as standalone Go binaries via JSON-RPC over stdin/stdout. Custom extensions can be written in Go, TypeScript, or Python.

```bash
make extensions              # Build all extension binaries
/ext-init my-extension       # Scaffold a new extension from within piglet
/extensions                  # List loaded extensions
```

Extensions can register tools, commands, shortcuts, prompt sections, interceptors, event handlers, and message hooks — all through the same API. See [`docs/extensions.md`](docs/extensions.md) for the full guide and [`examples/extensions/`](examples/extensions/) for working examples.

## License

MIT
