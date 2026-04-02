# Getting Started

- [Requirements](#requirements)
- [Installation](#installation)
- [API Keys](#api-keys)
- [First Launch](#first-launch)
- [Your First Conversation](#your-first-conversation)
- [One-Shot Mode](#one-shot-mode)
- [Next Steps](#next-steps)

## Requirements

- Go 1.26 or later
- Git (for extension installation)
- An API key from at least one supported [provider](providers.md)

## Installation

Install piglet with a single command:

```bash
go install github.com/dotcommander/piglet/cmd/piglet@latest
```

Verify the installation:

```bash
piglet --version
```

> **Building from source:** Clone the repository, then `go build -o piglet ./cmd/piglet/` and move the binary to a directory on your `$PATH`.

## API Keys

Piglet needs an API key from at least one LLM provider. The fastest way to get started is to export an environment variable:

```bash
# Anthropic (recommended)
export ANTHROPIC_API_KEY=sk-ant-...

# OpenAI
export OPENAI_API_KEY=sk-...

# Google
export GOOGLE_API_KEY=AIza...
```

Add the export to your shell profile (`~/.zshrc`, `~/.bashrc`, etc.) to make it permanent.

For other providers and advanced auth methods (secret managers, shell commands), see the [Providers](providers.md) documentation.

## First Launch

Run `piglet` with no arguments to start the interactive TUI:

```bash
piglet
```

On the very first launch, piglet runs an automatic setup:

1. Creates `~/.config/piglet/` with secure permissions
2. Writes a model catalog (`models.yaml`) covering all supported providers
3. Detects API keys from your environment
4. Writes `config.yaml` with your default model
5. Installs [extensions](extensions.md) in the background

You'll see output like this:

```
piglet — first-time setup

Creating ~/.config/piglet/...
  models.yaml ✓
Detected API keys: anthropic
  Default provider: anthropic (claude-opus-4-6)
  config.yaml ✓

Extensions will be installed automatically on first launch.

Setup complete! Run 'piglet' to start.
```

Run `piglet` again and the TUI appears immediately. Extensions build in the background while the UI loads — you can start typing right away.

## Your First Conversation

Type a message and press `Enter`. Piglet streams the response in real time.

A few things to try:

```
> explain this codebase
> read main.go and suggest improvements
> find all TODO comments in this project
```

Piglet has built-in tools for reading files, writing code, running shell commands, and searching — the model calls them automatically when needed.

### Useful Commands

Type `/help` to see all available commands. Here are the essentials:

| Command | Shortcut | Description |
|---------|----------|-------------|
| `/help` | | List all commands |
| `/model` | `Ctrl+P` | Switch model |
| `/session` | `Ctrl+S` | Switch or create session |
| `/clear` | | Clear conversation history |
| `/quit` | `Ctrl+C` | Exit piglet |

For the full command reference, see [Commands](commands.md).

## One-Shot Mode

Pass a prompt as an argument to get a single response without entering the TUI:

```bash
piglet "what files are in this directory?"
```

Use `--json` for machine-readable output:

```bash
piglet --json "list the exported functions in main.go"
```

## Next Steps

| Topic | Description |
|-------|-------------|
| [Configuration](configuration.md) | Customize settings, agent behavior, and tool limits |
| [Providers](providers.md) | Set up providers, configure auth, use local models |
| [Extensions](extensions.md) | Install, manage, and disable extensions |
| [Sessions](sessions.md) | Branching, forking, and conversation history |
| [Commands](commands.md) | All slash commands and keyboard shortcuts |
| [Architecture](architecture.md) | How piglet works under the hood |
