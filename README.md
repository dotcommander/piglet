<p align="center">
  <img src="docs/piglet.png" alt="piglet" width="300">
</p>

<h1 align="center">piglet</h1>

<p align="center">A coding assistant that lives in your terminal.</p>

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

For full functionality, install [extensions](https://github.com/dotcommander/piglet-extensions):

```bash
git clone https://github.com/dotcommander/piglet-extensions
cd piglet-extensions
make extensions   # build + install to ~/.config/piglet/extensions/
```

## Use it

Set an API key and run it. First launch auto-detects your keys, picks a model, and writes config to `~/.config/piglet/`.

```bash
export ANTHROPIC_API_KEY=sk-ant-...
piglet
```

One-shot mode -- answers and exits:

```bash
piglet "what does this function do"
piglet "refactor main.go to use errgroup"
```

It reads your actual files, runs real commands, and edits in place. No copy-pasting. Sessions persist -- come back days later and pick up exactly where you stopped.

## Models

Switch mid-session with `Ctrl+P`. No restart needed.

| Provider | Models | Key |
|----------|--------|-----|
| OpenAI | `gpt-5`, `gpt-5-mini`, `gpt-5.1`, `o4-mini` | `OPENAI_API_KEY` |
| Anthropic | `claude-sonnet-4-20250514`, `claude-haiku-4-5-20251001` | `ANTHROPIC_API_KEY` |
| Google | `gemini-2.5-pro`, `gemini-2.5-flash` | `GOOGLE_API_KEY` |
| xAI | `grok-3` | `XAI_API_KEY` |
| Groq | `llama-3.3-70b-versatile` | `GROQ_API_KEY` |
| OpenRouter | `auto` (routes best available) | `OPENROUTER_API_KEY` |
| LM Studio | `local-model` (localhost:1234) | -- |

Use just the model ID (`gpt-5`) or the full form (`openai/gpt-5`). Override at startup with `PIGLET_DEFAULT_MODEL` or `defaultModel` in config.

## Config

Everything in `~/.config/piglet/`:

| File | Purpose |
|------|---------|
| `config.yaml` | Model, theme, shell, extensions |
| `auth.json` | API keys -- literals, `$ENV_VAR`, or `!shell command` |
| `prompt.md` | Your system prompt. Overrides `systemPrompt` in config. |
| `behavior.md` | Behavioral guidelines |
| `sessions/` | Conversation history |
| `skills/` | Markdown methodology files |
| `prompts/` | Templates that become slash commands |

```yaml
# config.yaml
defaultProvider: anthropic
defaultModel: claude-sonnet-4-20250514
systemPrompt: "You are a senior Go developer. Always write tests."
theme: dark
shellPath: /bin/zsh
```

Auth supports env references and shell commands:

```json
{
  "openai": "$OPENAI_API_KEY",
  "anthropic": "sk-ant-...",
  "google": "!op read op://vault/google/key"
}
```

## Commands

| Command | Action |
|---------|--------|
| `/help` | Available commands |
| `/clear` | Clear conversation |
| `/step` | Toggle step-by-step mode |
| `/model` | Switch model |
| `/session` | List and load sessions |
| `/branch` | Fork current session |
| `/compact` | Summarize conversation to free context |
| `/search` | Search history |
| `/export` | Export session |
| `/undo` | Undo last file change |
| `/config` | Show or set up configuration |
| `/extensions` | List loaded extensions |
| `/bg` | Run a background agent |
| `/quit` | Exit |

| Key | Action |
|-----|--------|
| `Ctrl+C` | Stop streaming / quit |
| `Ctrl+P` | Model selector |
| `Ctrl+S` | Session picker |
| `Ctrl+V` | Paste clipboard image |
| `Enter` | Send |
| `Alt+Enter` | Newline |

## Extensions

Ten extensions run as standalone binaries from [`piglet-extensions`](https://github.com/dotcommander/piglet-extensions) (safeguard, rtk, autotitle, clipboard, skill, memory, subagent, lsp, repomap, plan):

```bash
/extensions            # list what's loaded
/ext-init my-tool      # scaffold a new one
```

Extensions register tools, commands, shortcuts, prompt sections, interceptors, and event handlers -- same API whether compiled-in or external. Write them in Go, TypeScript, or Python. See [`docs/extensions.md`](docs/extensions.md).

## License

MIT
