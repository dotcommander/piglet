<p align="center">
  <img src="docs/piglet.png" alt="piglet" width="300">
</p>

<h1 align="center">piglet</h1>

<p align="center">A minimalist TUI coding assistant with multi-provider LLM support and a built-in tool suite.</p>

## Features

- Interactive TUI built with Bubble Tea v2
- Print mode for one-shot queries: `piglet "explain this error"`
- Multi-provider: OpenAI, Anthropic, Google, xAI, Groq, OpenRouter
- Built-in tools: read, write, edit, bash, grep, find, ls
- Session persistence via JSONL — resume where you left off
- Conversation compaction to manage context window
- Customizable system prompt via config or `~/.config/piglet/prompt.md`
- Extension API: register custom tools, commands, shortcuts, prompt sections, and interceptors
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
```

## Quick Start

**Interactive mode** — opens a TUI session:

```bash
piglet
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
| `sessions/` | Persisted conversation history |

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

**Auth setup** — add an API key:

```bash
piglet auth set openai sk-...
piglet auth set anthropic sk-ant-...
piglet auth set google AIza...
```

Keys can also reference env vars or shell commands:

```json
{
  "openai": "$OPENAI_API_KEY",
  "anthropic": "!op read op://vault/anthropic/key"
}
```

**Environment variables:**

| Variable | Effect |
|----------|--------|
| `PIGLET_DEFAULT_MODEL` | Override model (e.g. `gpt-4o-mini`) |
| `OPENAI_API_KEY` | OpenAI key (auto-detected) |
| `ANTHROPIC_API_KEY` | Anthropic key (auto-detected) |
| `GOOGLE_API_KEY` | Google key (auto-detected) |

## Providers

| Provider | Example Models | Key Env Var |
|----------|---------------|-------------|
| OpenAI | `gpt-4o`, `gpt-4o-mini`, `o3-mini` | `OPENAI_API_KEY` |
| Anthropic | `claude-sonnet-4-20250514`, `claude-opus-4-20250514` | `ANTHROPIC_API_KEY` |
| Google | `gemini-2.5-pro`, `gemini-2.5-flash` | `GOOGLE_API_KEY` |
| xAI | `grok-3` | `XAI_API_KEY` |
| Groq | `llama-3.3-70b-versatile` | `GROQ_API_KEY` |
| OpenRouter | `auto` (routes best available) | `OPENROUTER_API_KEY` |

Specify a model with `PIGLET_DEFAULT_MODEL` or set `defaultModel` in config. You can use just the model ID (`gpt-4o`) or the full `provider/model-id` form (`openai/gpt-4o`).

## Commands

**Slash commands** (type in the input box):

| Command | Action |
|---------|--------|
| `/help` | Show available commands |
| `/clear` | Clear conversation history |
| `/step` | Toggle step-by-step mode |
| `/model` | Switch model |
| `/session` | List and load sessions |
| `/compact` | Summarize conversation to free context |
| `/export` | Export current session |
| `/quit` | Exit |

All built-in commands register through the extension API and can be overridden.

**Keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `Ctrl+C` | Stop streaming / quit |
| `Ctrl+P` | Open model selector |
| `Ctrl+S` | Open session picker |
| `Enter` | Send message |
| `Shift+Enter` | Insert newline |

Shortcuts also register through the extension API and can be customized.

## Extensions

Piglet supports extensions for custom tools, slash commands, keyboard shortcuts, prompt sections, and message interceptors. Extensions register against the `ext.App` interface:

```go
app.RegisterTool(ext.ToolDef{
    Name:        "my-tool",
    Description: "Does something useful",
    Parameters:  schema,
    Handler:     myHandler,
})

app.RegisterPromptSection(ext.PromptSection{
    Title:   "Project Rules",
    Content: "Always use structured logging.",
    Order:   10,
})
```

See [`examples/extensions/`](examples/extensions/) for working examples:

- `git-tool/` — git operations as a tool
- `quicknotes/` — session note-taking

## License

MIT
