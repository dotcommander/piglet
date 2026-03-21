# Configuration

Piglet stores configuration in `~/.config/piglet/`.

## Settings

**File:** `~/.config/piglet/config.yaml`

```yaml
defaultProvider: openai
defaultModel: gpt-4o
systemPrompt: "You are piglet, a helpful coding assistant."
theme: dark
shellPath: /bin/zsh
extensions:
  - ~/my-extensions/custom-tools.so
providers:
  openai: https://api.openai.com     # default, override for proxies
```

### Settings Reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `defaultProvider` | string | `""` | Preferred provider |
| `defaultModel` | string | `gpt-4o` | Model ID or `provider/model-id` |
| `systemPrompt` | string | `"You are piglet, a helpful coding assistant."` | Base identity for the LLM |
| `theme` | string | `dark` | Color theme |
| `shellPath` | string | system default | Shell for bash tool |
| `extensions` | list | `[]` | Extension paths to load |
| `providers` | map | `{}` | Base URL overrides per provider |

### Environment Variables

Environment variables take precedence over config file settings.

| Variable | Effect |
|----------|--------|
| `PIGLET_DEFAULT_MODEL` | Override default model |
| `PIGLET_DEFAULT_PROVIDER` | Override default provider |
| `PIGLET_THEME` | Override theme |
| `PIGLET_SHELL_PATH` | Override shell path |

## System Prompt

The system prompt controls how the LLM behaves. It's built from multiple sources in priority order:

1. **`~/.config/piglet/prompt.md`** — full custom prompt file (highest priority)
2. **`systemPrompt` in config.yaml** — one-liner identity
3. **Built-in default** — "You are piglet, a helpful coding assistant."

After the base identity, the prompt builder appends:
- Extension-registered prompt sections (via `ext.RegisterPromptSection`)
- Tool hints and guidelines from all registered tools

**Example `~/.config/piglet/prompt.md`:**

```markdown
You are piglet, a senior Go developer assistant.

# Rules
- Always write table-driven tests
- Use errgroup over sync.WaitGroup
- Prefer slog for logging
- Follow the project's CLAUDE.md conventions
```

## Authentication

**File:** `~/.config/piglet/auth.json`

API keys are stored as a JSON object mapping provider names to keys:

```json
{
  "openai": "sk-...",
  "anthropic": "sk-ant-...",
  "google": "AIza..."
}
```

### Key Resolution Order

1. Direct value in `auth.json`
2. Environment variable reference: `"$OPENAI_API_KEY"`
3. Shell command: `"!op read op://vault/anthropic/key"`
4. Auto-detected environment variables (e.g., `OPENAI_API_KEY`)

### Supported Provider Keys

| Provider | Env Variable |
|----------|-------------|
| `openai` | `OPENAI_API_KEY` |
| `anthropic` | `ANTHROPIC_API_KEY` |
| `google` | `GOOGLE_API_KEY` |
| `xai` | `XAI_API_KEY` |
| `groq` | `GROQ_API_KEY` |
| `openrouter` | `OPENROUTER_API_KEY` |

## Sessions

**Directory:** `~/.config/piglet/sessions/`

Sessions are stored as JSONL files, one per conversation. Each line is a JSON object with `type`, `ts`, and `data` fields.

Sessions are created automatically when you start piglet. Use `/session` or `Ctrl+S` to switch between sessions.
