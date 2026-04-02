# Commands & Shortcuts

- [Built-In Commands](#built-in-commands)
- [Extension Commands](#extension-commands)
- [Keyboard Shortcuts](#keyboard-shortcuts)
- [Immediate Commands](#immediate-commands)
- [Custom Shortcuts](#custom-shortcuts)
- [Prompt Templates](#prompt-templates)
- [Input Queue](#input-queue)

## Built-In Commands

These commands are compiled into the piglet binary and always available:

| Command | Description |
|---------|-------------|
| `/help` | List all available commands and shortcuts |
| `/clear` | Clear the conversation history |
| `/compact` | Compact conversation to reduce token usage |
| `/step` | Toggle step-by-step tool approval mode |
| `/model` | Open the model selector |
| `/session` | Open the session picker |
| `/session new` | Create a new blank session |
| `/search <query>` | Search across sessions |
| `/title <text>` | Set the current session title |
| `/branch` | Open the branch picker for in-place branching |
| `/fork` | Fork the current session to a new file |
| `/tree` | Display the session tree structure |
| `/bg <prompt>` | Run a prompt in the background agent |
| `/bg-cancel` | Cancel the running background agent |
| `/update` | Upgrade piglet binary and rebuild extensions |
| `/upgrade` | Alias for `/update` |
| `/quit` | Exit piglet |

### /help

Lists all registered commands (built-in and extension) with their descriptions, plus keyboard shortcuts.

### /clear

Clears the conversation history. The session file is preserved — clearing only affects the in-memory state. Start a new conversation in the same session.

### /compact

Manually triggers conversation compaction. Requires at least 4 messages. If an extension provides a compactor (e.g., pack-context's LLM-based compactor), it produces an intelligent summary. Otherwise, piglet keeps the first message plus the 6 most recent.

### /step

Toggles step mode. When enabled, the agent pauses before each tool call and shows an overlay asking you to approve, skip, or abort. Useful for reviewing what the agent wants to do before it does it.

### /model

Opens a filterable picker showing all available models. Type to filter, arrow keys to navigate, Enter to select. The model switch takes effect immediately for the next message.

**Shortcut:** `Ctrl+P`

### /session

Opens the session picker showing all sessions, newest first. Each entry shows the title, model, working directory, and message count. Select a session to switch to it.

Pass `new` to create a blank session linked to the current one:

```
/session new
```

**Shortcut:** `Ctrl+S`

### /search

Search across all sessions by title or content:

```
/search auth refactor
```

Results open in a picker. Select one to switch to that session.

### /title

Set a custom title for the current session:

```
/title Refactoring the auth middleware
```

Overrides the auto-generated title.

### /branch

Opens a picker showing all entries on the current branch. Select an entry to branch from that point — the conversation rewinds and new messages continue from there. The abandoned path is preserved in the tree. See [Sessions — Branching](sessions.md#branching).

### /fork

Forks the current session to a new file, copying all messages. The new session is independent but linked to its parent. See [Sessions — Forking](sessions.md#forking).

### /tree

Displays the tree structure of the current session in ASCII art, showing branch points, summaries, and the active path.

### /bg

Runs a prompt in a separate background agent while you continue working:

```
/bg analyze the test coverage and suggest improvements
```

The background agent has access to read-only tools (marked `BackgroundSafe`). Results appear when the background agent finishes. Cancel with `/bg-cancel`.

### /update

Upgrades the piglet binary to the latest release and rebuilds all extensions from the latest source. Equivalent to running `piglet update` from the command line.

Pass `--local` to build extensions from the local checkout instead of cloning from GitHub (for development):

```
/update --local
```

## Extension Commands

Extensions register additional commands. Here are the commands provided by the official extension packs:

### pack-core

| Command | Description |
|---------|-------------|
| `/export` | Export conversation to markdown |
| `/undo` | Undo the last file change |
| `/extensions` | List loaded extensions and their registrations |
| `/ext-init <name>` | Scaffold a new extension |

### pack-context

| Command | Description |
|---------|-------------|
| `/memory` | Manage memory entries |
| `/skill` | Manage skill files |
| `/inbox` | View and manage inbox items |
| `/behavior` | Show current behavior guidelines |
| `/prompts` | List registered prompt templates |
| `/session-tools` | Session annotation tools |

### pack-agent

| Command | Description |
|---------|-------------|
| `/provider` | Switch LLM provider |
| `/loop` | Run a recurring prompt on an interval |
| `/dispatch` | Dispatch a sub-agent |

### pack-workflow

| Command | Description |
|---------|-------------|
| `/pipe` | Run a multi-step pipeline |
| `/bulk` | Run a command across multiple directories |
| `/usage` | Show token usage statistics |

### pack-cron

| Command | Description |
|---------|-------------|
| `/cron` | Manage scheduled tasks (add, remove, list, etc.) |

### mcp

| Command | Description |
|---------|-------------|
| `/mcp` | Manage MCP server connections |

> The exact commands available depend on which extensions are installed and enabled. Run `/help` to see the current list.

## Keyboard Shortcuts

### Global Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+C` | Stop the running agent, or quit if idle |
| `Ctrl+P` | Open model selector |
| `Ctrl+S` | Open session picker |
| `Ctrl+M` | Toggle mouse mode (scroll wheel support) |
| `Ctrl+Z` | Suspend piglet (return to shell) |
| `Page Up` | Scroll conversation up |
| `Page Down` | Scroll conversation down |
| `Enter` | Send message or execute command |
| `Up Arrow` | Previous input from history |
| `Down Arrow` | Next input from history |

### Step Mode Shortcuts

When step mode is active and a tool call is pending:

| Shortcut | Action |
|----------|--------|
| `Enter` / `y` | Approve the tool call |
| `s` | Skip this tool call |
| `Esc` / `n` | Abort all pending tool calls |

### Extension Shortcuts

Extensions can register additional shortcuts. The pack-agent extension registers:

| Shortcut | Action |
|----------|--------|
| `Ctrl+G` | Show git status (if configured) |

## Immediate Commands

Most commands wait until the agent finishes before executing. Commands marked as **immediate** can run while the agent is streaming. This is useful for commands that don't modify conversation state.

Built-in immediate commands: none by default (all built-in commands wait for the agent).

Extensions can register immediate commands by setting `Immediate: true` in the command definition.

## Custom Shortcuts

Override the default keyboard bindings in `config.yaml`:

```yaml
shortcuts:
  model: ctrl+p          # Default
  session: ctrl+s        # Default
```

Extensions may register additional shortcut actions. The value format follows Bubble Tea's key binding syntax: `ctrl+x`, `alt+x`, `shift+tab`, etc.

## Prompt Templates

Prompt templates turn markdown files into slash commands. They're a lightweight alternative to full extensions.

### Creating a Template

Create a file in `~/.config/piglet/prompts/`:

```markdown
---
description: Review code for bugs and style issues
---
Review the following code for bugs, security vulnerabilities, and style problems.
Focus on correctness first, then readability.

$@
```

The filename (minus `.md`) becomes the command name: `review.md` → `/review`.

### Using Templates

```
/review the authentication middleware
```

The `$@` placeholder is replaced with your arguments.

### Argument Placeholders

| Placeholder | Meaning |
|-------------|---------|
| `$1`, `$2`, ... `$9` | Individual positional arguments |
| `$@` | All arguments joined by space |
| `${@:N}` | Arguments from position N onward |
| `${@:N:L}` | L arguments starting from position N |

### Template Locations

| Location | Scope | Priority |
|----------|-------|----------|
| `.piglet/prompts/` | Project-local | Higher (overrides global) |
| `~/.config/piglet/prompts/` | Global | Lower |

## Input Queue

When the agent is streaming and you type a message or command, it's queued and executed after the current turn finishes. A "Queued" notification appears in the status bar.

You can queue multiple messages — they're processed in order. The queue is visible in the status bar when items are pending.
