# Models

Piglet includes a built-in model catalog. Switch models at any time with `/model` or `Ctrl+P`.

## Built-in Models

Piglet ships with a default model catalog covering major providers. The catalog is written to `~/.config/piglet/models.yaml` on first run and can be refreshed by deleting the file and restarting.

To see all available models, run `/model` or press `Ctrl+P` inside a session.

| Provider | Example Models | Env Variable |
|----------|----------------|--------------|
| Anthropic | Claude Opus, Sonnet, Haiku | `ANTHROPIC_API_KEY` |
| OpenAI | GPT-5, o4-mini, o3 | `OPENAI_API_KEY` |
| Google | Gemini 2.5 Pro/Flash | `GOOGLE_API_KEY` |
| xAI | Grok 3 | `XAI_API_KEY` |
| Groq | Llama 3.3 70B | `GROQ_API_KEY` |
| OpenRouter | Auto (routes best available) | `OPENROUTER_API_KEY` |
| Z.AI | GLM-5, GLM-4.7 | `ZAI_API_KEY` |
| LM Studio | Any local model | — |

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

Any provider that implements the OpenAI streaming interface works via base URL override in `config.yaml`.

## Custom Providers

Override provider base URLs for proxies or self-hosted endpoints:

```yaml
# ~/.config/piglet/config.yaml
providers:
  openai: https://my-proxy.example.com
```

Models using the OpenAI-compatible API format work with any endpoint that implements the same streaming interface.
