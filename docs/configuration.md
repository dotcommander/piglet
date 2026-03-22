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
| `defaultModel` | string | `""` | Model ID or `provider/model-id` |
| `systemPrompt` | string | `""` | Base identity (overridden by `prompt.md`) |
| `theme` | string | `""` | Color theme |
| `shellPath` | string | system default | Shell for bash tool |
| `extensions` | list | `[]` | Extension paths to load |
| `providers` | map | `{}` | Base URL overrides per provider |
| `shortcuts` | map | `{}` | Action → keybind (e.g. `model: ctrl+p`) |
| `promptOrder` | map | `{}` | Prompt section title → order override |
| `projectDocs` | list | `[]` | Files to auto-read for context (see below) |
| `rtk` | bool | auto-detect | Enable/disable RTK token optimization |
| `debug` | bool | `false` | Log all request/response payloads |
| `safeguard` | bool | `true` | Enable/disable dangerous command blocking |

#### Agent Settings (`agent:`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `maxTurns` | int | `10` | Max agent turns per interaction |
| `bgMaxTurns` | int | `5` | Max turns for background agents |
| `autoTitle` | bool | `true` | Auto-generate session titles |
| `compactKeepRecent` | int | `6` | Messages to keep after compaction |
| `compactAt` | int | `0` | Token threshold for auto-compact (0 = disabled) |
| `maxMessages` | int | `200` | Hard cap on conversation messages |
| `maxTokens` | int | model default | Output token limit |
| `maxRetries` | int | `3` | Retry attempts on error |
| `toolConcurrency` | int | `10` | Max parallel tool calls |

#### Git Context Settings (`git:`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `maxDiffStatFiles` | int | `30` | Max files in diff stat |
| `maxLogLines` | int | `5` | Recent commit lines in prompt |
| `maxDiffHunkLines` | int | `50` | Max diff hunk lines |
| `commandTimeout` | int | `5` | Git command timeout (seconds) |

#### Tool Settings (`tools:`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `readLimit` | int | `2000` | Max lines per read |
| `grepLimit` | int | `100` | Max grep matches |

#### Bash Settings (`bash:`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `defaultTimeout` | int | `30` | Default command timeout (seconds) |
| `maxTimeout` | int | `300` | Maximum allowed timeout (seconds) |
| `maxStdout` | int | `100000` | Max stdout bytes |
| `maxStderr` | int | `50000` | Max stderr bytes |

#### Sub-Agent Settings (`subagent:`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `maxTurns` | int | `10` | Max turns for sub-agents |

#### Project Docs (`projectDocs:`)

Auto-read files into the system prompt as context:

```yaml
projectDocs:
  - name: CLAUDE.md
    title: Project Instructions
  - name: docs/architecture.md
    title: Architecture
```

### Environment Variables

| Variable | Effect |
|----------|--------|
| `PIGLET_DEFAULT_MODEL` | Override default model |

## Prompt Templates

**Directories:** `~/.config/piglet/prompts/` (global), `.piglet/prompts/` (project-local)

Prompt templates are markdown files that register as slash commands. The filename (minus `.md`) becomes the command name.

**Example** — create `~/.config/piglet/prompts/review.md`:

```markdown
---
description: Review code for issues
---
Review the following code for bugs, security issues, and style problems:

$@
```

Now `/review fix the auth bug` expands the template with your args and sends it.

### Arg Substitution

| Placeholder | Meaning |
|-------------|---------|
| `$1`, `$2`, ... `$9` | Positional args |
| `$@` | All args joined by space |
| `${@:N}` | Args from position N onward (1-indexed) |
| `${@:N:L}` | L args starting from position N |

Project-local templates (`.piglet/prompts/`) override global templates when names collide. Missing args resolve to empty strings.

### YAML Frontmatter

Optional. Only `description` is recognized — it appears in `/help` output.

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
