# Models

Piglet includes a built-in model catalog. Switch models at any time with `/model` or `Ctrl+P`.

## Built-in Models

| Provider | Model ID | Display Name | Context | Max Output |
|----------|----------|--------------|---------|------------|
| OpenAI | `gpt-4o` | GPT-4o | 128K | 16K |
| OpenAI | `gpt-4o-mini` | GPT-4o Mini | 128K | 16K |
| OpenAI | `o3-mini` | o3-mini | 200K | 100K |
| Anthropic | `claude-sonnet-4-20250514` | Claude Sonnet 4 | 200K | 8K |
| Anthropic | `claude-opus-4-20250514` | Claude Opus 4 | 200K | 8K |
| Anthropic | `claude-haiku-3-5-20241022` | Claude 3.5 Haiku | 200K | 8K |
| Google | `gemini-2.5-pro` | Gemini 2.5 Pro | 1M | 64K |
| Google | `gemini-2.5-flash` | Gemini 2.5 Flash | 1M | 64K |
| xAI | `grok-3` | Grok 3 | 128K | 16K |
| Groq | `llama-3.3-70b-versatile` | Llama 3.3 70B | 128K | 8K |
| OpenRouter | `auto` | Auto | 200K | 16K |

## Selecting a Model

**At startup:**

```bash
# Environment variable
PIGLET_DEFAULT_MODEL=claude-sonnet-4-20250514 piglet

# Config file (~/.config/piglet/config.yaml)
defaultModel: gemini-2.5-pro
```

**During a session:**

- Type `/model` or press `Ctrl+P` to open the model selector
- Filter by typing, navigate with arrow keys, select with Enter

## Model Resolution

When you specify a model, piglet resolves it in this order:

1. Exact match: `openai/gpt-4o`
2. Model ID match: `gpt-4o` (searches all providers)
3. Prefix match: `gpt-4` (matches first model starting with that prefix)

## Custom Providers

Override provider base URLs for proxies or self-hosted endpoints:

```yaml
# ~/.config/piglet/config.yaml
providers:
  openai: https://my-proxy.example.com
```

Models using the OpenAI-compatible API format work with any endpoint that implements the same streaming interface.
