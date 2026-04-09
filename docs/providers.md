# Providers

- [Overview](#overview)
- [Authentication](#authentication)
- [Supported Providers](#supported-providers)
- [Selecting a Model](#selecting-a-model)
- [Model Resolution](#model-resolution)
- [Local Models](#local-models)
- [Custom Endpoints](#custom-endpoints)
- [Model Catalog](#model-catalog)
- [Streaming Protocols](#streaming-protocols)

## Overview

Piglet is provider-agnostic. It supports multiple LLM providers through the OpenAI-compatible streaming protocol (built-in) plus extension-provided protocols for Anthropic and Google. You can switch between providers mid-session. Every provider works the same way — set an API key, pick a model, and go.

## Authentication

Piglet resolves API keys in this order (first match wins):

1. **Runtime override** — set programmatically during the session
2. **Stored credential** in `~/.config/piglet/auth.json`
3. **Environment variable** — e.g., `ANTHROPIC_API_KEY`

### Environment Variables

The simplest way to authenticate. Add to your shell profile:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...
export GOOGLE_API_KEY=AIza...
```

### auth.json

For persistent, file-based configuration:

```json
{
  "anthropic": "sk-ant-...",
  "openai": "sk-...",
  "google": "AIza..."
}
```

Values in `auth.json` support three formats:

| Format | Example | Description |
|--------|---------|-------------|
| Literal | `"sk-ant-..."` | Plain API key |
| Env reference | `"$ANTHROPIC_API_KEY"` or `"${ANTHROPIC_API_KEY}"` | Resolved from environment at runtime |
| Shell command | `"!op read op://vault/anthropic/key"` | Executed via `sh -c`; stdout is the key (10s timeout) |

The shell command format is useful for secret managers like 1Password, Vault, or AWS Secrets Manager:

```json
{
  "anthropic": "!op read op://Personal/Anthropic/credential",
  "openai": "!aws secretsmanager get-secret-value --secret-id openai-key --query SecretString --output text"
}
```

### Provider Aliases

Piglet normalizes provider names. These aliases resolve automatically:

| Alias | Canonical Name |
|-------|---------------|
| `gemini` | `google` |
| `vertex` | `google-vertex` |
| `bedrock` | `amazon-bedrock` |
| `copilot` | `github-copilot` |
| `azure` | `azure-openai` |

## Supported Providers

| Provider | API Key Variable | Protocol | Notes |
|----------|-----------------|----------|-------|
| **Anthropic** | `ANTHROPIC_API_KEY` | Anthropic (via extension) | Prompt caching enabled automatically |
| **OpenAI** | `OPENAI_API_KEY` | OpenAI | Supports `max_completion_tokens` for newer models |
| **Google** | `GOOGLE_API_KEY` or `GEMINI_API_KEY` | Google (via extension) | Gemini models via `generativelanguage.googleapis.com` |
| **xAI** | `XAI_API_KEY` | OpenAI | Grok models via OpenAI-compatible API |
| **Groq** | `GROQ_API_KEY` | OpenAI | Fast inference for open models |
| **OpenRouter** | `OPENROUTER_API_KEY` | OpenAI | Routes to best available model |
| **Z.AI** | `ZAI_API_KEY` | OpenAI | GLM models |
| **LM Studio** | — | OpenAI | Local models, no API key needed |
| **Ollama** | — | OpenAI | Local models, no API key needed |

### Anthropic

The Anthropic streaming protocol is provided by the `pack-agent` extension. It automatically enables prompt caching:

- The system prompt is wrapped in cacheable blocks
- Tools are cached with a breakpoint on the last tool
- Conversation history gets a cache breakpoint on the 2nd-to-last user message

This happens transparently — no configuration needed. Requires the `pack-agent` extension to be installed (`/extensions install`).

### OpenAI

Piglet auto-detects whether a model requires `max_completion_tokens` (newer models like GPT-5, o3, o4) vs. `max_tokens` (older models). The `/v1` endpoint suffix is managed automatically.

### Google

The Google streaming protocol is provided by the `pack-agent` extension. Uses the `v1beta` streaming endpoint with Server-Sent Events. Supports larger SSE buffers (256KB initial, 10MB max) for models that produce long outputs. Requires the `pack-agent` extension to be installed (`/extensions install`).

## Selecting a Model

### At Startup

```bash
# CLI flag (highest priority)
piglet --model claude-opus-4-6

# Environment variable
PIGLET_DEFAULT_MODEL=gpt-5 piglet

# Config file (~/.config/piglet/config.yaml)
defaultModel: claude-opus-4-6
```

### During a Session

Press `Ctrl+P` or type `/model` to open the model selector. Filter by typing, navigate with arrow keys, select with Enter. The model switch takes effect immediately for the next message.

### URL-as-Model

Connect to any local or remote OpenAI-compatible server directly:

```bash
# Shorthand for localhost
piglet --model :8080

# Full URL
piglet --model http://192.168.1.5:8080
```

## Model Resolution

When you specify a model by name, piglet resolves it through these steps:

1. **Exact match** — `openai/gpt-5` matches the full `provider/model-id` key
2. **ID match** — `gpt-5` searches across all providers
3. **Prefix match** — `gpt-5` matches the first model whose ID starts with that string
4. **Substring match** — `opus` matches any model containing that string

This means you can use short names like `opus`, `sonnet`, `gpt-5`, or `gemini-pro` and piglet will find the right model.

## Local Models

Piglet works with any server that implements the OpenAI `/v1/chat/completions` streaming interface.

### Ad-Hoc Connection

The fastest way to use a local model — no configuration needed:

```bash
piglet --model :8080          # localhost shorthand
piglet --model :1234          # LM Studio default port
piglet --model :11434         # Ollama default port
```

Piglet automatically:

1. **Probes** `GET /v1/models` to discover running models
2. **Detects** the server type (LM Studio, Ollama, or generic)
3. **Registers** the model so it appears in the `/model` picker
4. **Skips auth** — sends `Bearer local` for localhost servers

### Persistent Configuration

To auto-discover local servers on every startup, add them to `config.yaml`:

```yaml
localServers:
  - http://localhost:1234     # LM Studio
  - http://localhost:11434    # Ollama

localDefaults:
  contextWindow: 8192         # Fallback context window
  maxTokens: 4096             # Fallback max output tokens
```

All models from configured servers appear in the model picker automatically.

### Supported Servers

| Server | Default Port | Auto-Detected | Notes |
|--------|-------------|---------------|-------|
| LM Studio | 1234 | Yes | Detected via `owned_by` field in model response |
| Ollama | 11434 | Yes | Detected via response headers |
| vLLM | 8000 | Yes (generic) | |
| llama.cpp | 8080 | Yes (generic) | |
| MLX | 8080 | Yes (generic) | MLX Serve, mlx-lm |

Piglet filters out embedding models automatically — models with names containing `embed`, `minilm`, `e5-`, `bge-`, or `gte-` are excluded from the model picker.

If auto-discovery fails (server not running, non-standard API), piglet falls back to a generic "local" model with reasonable defaults.

### Progressive Tool Disclosure

Local models often have smaller context windows than cloud models. To keep the prompt manageable, piglet uses **progressive tool disclosure** for local model connections.

**How it works:**

When connected to a local model, piglet switches from `ToolModeFull` (all 49 tool schemas in the prompt) to `ToolModeCompact`:

- **7 core tools** (`read`, `write`, `edit`, `bash`, `grep`, `find`, `ls`) send their full parameter schemas — always available
- **42 extension tools** send only their name and one-line description — no parameter schemas
- The deferred tools appear in a prompt section titled "Available Tools" so the model knows they exist
- When the model needs a deferred tool, it calls `tool_search` with the tool name
- `tool_search` returns the full schema and **activates** the tool — subsequent turns include the full schema automatically

This reduces prompt size significantly while keeping all tools accessible on demand.

**Example flow:**

```
User: "search my memory for auth patterns"

Model sees: memory_search (Search stored memory entries) — no parameters shown
Model calls: tool_search("memory_search")
Piglet returns: full schema with parameters (query, limit, tags, etc.)
Model calls: memory_search(query="auth patterns")
```

After activation, `memory_search` stays fully expanded for the rest of the session.

**Configuration:**

Tool mode is set automatically based on the provider — local servers use compact mode, cloud providers use full mode. You can customize the instruction shown to the model for deferred tools:

```yaml
# ~/.config/piglet/config.yaml
deferredToolsNote: "Call tool_search before using any tool listed below."
```

Cloud providers always use full mode regardless of this setting.

## Custom Endpoints

Override any provider's base URL to point at a proxy, gateway, or self-hosted instance:

```yaml
# ~/.config/piglet/config.yaml
providers:
  openai: https://my-proxy.example.com/v1
  anthropic: https://anthropic-gateway.internal
```

This is useful for:

- Corporate proxies that route API traffic
- Self-hosted inference servers
- API gateways with custom auth headers

Custom headers can be set per-model in `models.yaml` for advanced configurations.

## Model Catalog

Piglet ships with a built-in model catalog written to `~/.config/piglet/models.yaml` on first run. This file defines every model's:

- ID and display name
- Provider and API protocol
- Base URL
- Context window and max output tokens
- Cost per million tokens (input, output, cache read, cache write)

### Refreshing the Catalog

Delete `models.yaml` and restart piglet to regenerate it with the latest defaults:

```bash
rm ~/.config/piglet/models.yaml
piglet  # Regenerates on startup
```

### Customizing Models

Edit `models.yaml` to add custom models, adjust costs, or override context windows:

```yaml
- id: my-finetuned-model
  name: My Fine-Tuned GPT
  provider: openai
  api: openai
  baseUrl: https://api.openai.com
  contextWindow: 128000
  maxTokens: 16384
  cost:
    input: 5.0
    output: 15.0
```

## Streaming Protocols

The core ships with one streaming protocol. Non-OpenAI protocols are provided by extensions via `RegisterStreamProvider`.

| Protocol | Used By | Wire Format | Source |
|----------|---------|-------------|--------|
| **OpenAI** | OpenAI, xAI, Groq, OpenRouter, Z.AI, local servers | `POST /v1/chat/completions` with `stream: true`, SSE | Built-in (`provider/`) |
| **Anthropic** | Anthropic | `POST /v1/messages` with `stream: true`, SSE | `pack-agent` extension |
| **Google** | Google Gemini | `POST /v1beta/models/{id}:streamGenerateContent?alt=sse` | `pack-agent` extension |

Any server that speaks the OpenAI protocol works out of the box. Anthropic and Google require the `pack-agent` extension.
