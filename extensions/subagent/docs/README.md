# Subagent

Delegates tasks to independent sub-agents that run to completion with their own LLM context, enabling focused execution for research, analysis, or complex tasks.

## Capabilities

| Capability | Name | Description |
|------------|------|-------------|
| tool | `dispatch` | Launch a sub-agent with a task |

## Tool Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task` | string | yes | Task instructions for the sub-agent |
| `context` | string | no | Additional context appended to system prompt |
| `tools` | enum | no | `read_only` (default) or `all` |
| `max_turns` | int | no | Max agent turns (default: 10) |
| `model` | string | no | Model override (e.g., `anthropic/claude-haiku-4-5`) |
| `prefer` | enum | no | `default` or `small` model preference |

## Depth Guard

Subagents are protected against runaway nesting. By default, nesting is limited to **2 levels** (main → subagent → sub-subagent). Deeper calls are blocked with an error.

| Env Var | Default | Description |
|---------|---------|-------------|
| `PIGLET_SUBAGENT_MAX_DEPTH` | 2 | Maximum nesting depth |
| `PIGLET_SUBAGENT_DEPTH` | (internal) | Current depth, auto-propagated |

To allow deeper nesting (use with caution):
```bash
export PIGLET_SUBAGENT_MAX_DEPTH=3
```

## How It Works

1. Resolves LLM provider from auth config (respects `prefer` for model selection)
2. Retrieves host tools — filtered to background-safe tools for `read_only`, all tools for `all`
3. Creates a new agent with the selected provider, tools, and system prompt from `~/.config/piglet/subagent.md`
4. Runs the agent's event loop, collecting token usage and output per turn
5. Returns formatted output with turn count and token statistics

## Configuration

| File | Purpose |
|------|---------|
| `~/.config/piglet/subagent.md` | System prompt for sub-agents (cached at startup) |

## Tool Access Modes

| Mode | Available Tools |
|------|-----------------|
| `read_only` | Only background-safe tools (read, grep, glob, etc.) |
| `all` | All host tools including write, edit, bash |
