# Autotitle

Automatically generates a short session title after the first user-assistant exchange.

## Capabilities

| Capability | Name | Priority |
|------------|------|----------|
| eventHandler | `EventAgentEnd` | 100 |

## How It Works

1. Listens for `EventAgentEnd` — fires once per session (atomic bool gate)
2. Checks the conversation has at least 2 messages and no existing title
3. Extracts the first user message and first assistant response (truncated to 200 runes each)
4. Calls the configured LLM with the system prompt from `~/.config/piglet/autotitle.md`
5. Caps the result at 50 runes and sets it via `ActionSetSessionTitle`

## Configuration

| File | Purpose |
|------|---------|
| `~/.config/piglet/autotitle.md` | System prompt for title generation |

The extension uses the configured "small" model with a 30-token max and 10-second timeout.

## Failure Behavior

Fails silently — returns an empty string if the prompt file is missing, the provider is unavailable, or the LLM call times out. Never blocks the session.
