# Models

Piglet includes a built-in model catalog. Switch models at any time with `/model` or `Ctrl+P`.

## Built-in Models

| Provider | Model ID | Display Name | Context | Max Output |
|----------|----------|--------------|---------|------------|
| Anthropic | `claude-opus-4-6` | Claude Opus 4.6 | 1M | 128K |
| Anthropic | `claude-sonnet-4-6` | Claude Sonnet 4.6 | 1M | 64K |
| Anthropic | `claude-sonnet-4-20250514` | Claude Sonnet 4 | 200K | 64K |
| Anthropic | `claude-haiku-4-5-20251001` | Claude Haiku 4.5 | 200K | 64K |
| OpenAI | `gpt-5.4` | GPT-5.4 | 1M | 128K |
| OpenAI | `gpt-5` | GPT-5 | 400K | 128K |
| OpenAI | `o4-mini` | o4-mini | 200K | 100K |
| OpenAI | `gpt-4.1` | GPT-4.1 | 1M | 32K |
| OpenAI | `gpt-4.1-mini` | GPT-4.1 mini | 1M | 32K |
| OpenAI | `gpt-4o` | GPT-4o | 128K | 16K |
| OpenAI | `o3` | o3 | 200K | 100K |
| Google | `gemini-3.1-pro-preview` | Gemini 3.1 Pro Preview | 1M | 64K |
| Google | `gemini-2.5-pro` | Gemini 2.5 Pro | 1M | 64K |
| Google | `gemini-2.5-flash` | Gemini 2.5 Flash | 1M | 64K |
| xAI | `grok-3` | Grok 3 | 128K | 8K |
| Groq | `llama-3.3-70b-versatile` | Llama 3.3 70B | 128K | 32K |
| OpenRouter | `auto` | Auto (best available) | 200K | 16K |
| Z.AI | `glm-5` | GLM-5 | 128K | 8K |
| Z.AI | `glm-4.7` | GLM-4.7 | 128K | 8K |
| Z.AI | `glm-5-turbo` | GLM-5 Turbo | 128K | 8K |
| LM Studio | `local-model` | Local Model | 32K | 32K |

## Selecting a Model

**At startup:**

```bash
# Environment variable
PIGLET_DEFAULT_MODEL=claude-opus-4-6 piglet

# Config file (~/.config/piglet/config.yaml)
defaultModel: gemini-2.5-pro
```

**During a session:**

- Type `/model` or press `Ctrl+P` to open the model selector
- Filter by typing, navigate with arrow keys, select with Enter

## Model Resolution

When you specify a model, piglet resolves it in this order:

1. Exact match: `openai/gpt-5`
2. Model ID match: `gpt-5` (searches all providers)
3. Prefix match: `gpt-5` (matches first model starting with that prefix)

## Providers

Piglet implements three streaming API protocols:

| Protocol | Native Providers | OpenAI-Compatible Providers |
|----------|------------------|-----------------------------|
| OpenAI | OpenAI | OpenRouter, xAI/Grok, Groq, Z.AI, LM Studio, Ollama, any `/v1/chat/completions` endpoint |
| Anthropic | Anthropic | — |
| Google | Google (Gemini) | — |

Any provider that implements the OpenAI streaming interface works via base URL override. This covers the majority of providers available through [charmbracelet/fantasy](https://github.com/charmbracelet/fantasy)'s `openaicompat` layer.

Fantasy's dedicated provider packages (for reference): `openai`, `anthropic`, `google`, `openrouter`, `azure`, `bedrock`, `vercel`, `openaicompat`.

## Custom Providers

Override provider base URLs for proxies or self-hosted endpoints:

```yaml
# ~/.config/piglet/config.yaml
providers:
  openai: https://my-proxy.example.com
```

Models using the OpenAI-compatible API format work with any endpoint that implements the same streaming interface.
