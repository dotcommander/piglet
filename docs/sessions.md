# Sessions

- [Overview](#overview)
- [Session Lifecycle](#session-lifecycle)
- [Switching Sessions](#switching-sessions)
- [Branching](#branching)
- [Forking](#forking)
- [Session Tree](#session-tree)
- [Compaction](#compaction)
- [Labels](#labels)
- [Searching](#searching)
- [Auto-Titling](#auto-titling)
- [Storage Format](#storage-format)

## Overview

Every conversation in piglet is a session. Sessions persist to disk as JSONL files in `~/.config/piglet/sessions/`, so you can close piglet and resume exactly where you left off.

Sessions support **tree-structured branching** — you can go back to any point in a conversation and try a different approach without losing the original path. This is essential for exploratory coding workflows.

## Session Lifecycle

A new session is created automatically when you start piglet. Each session records:

- A unique ID
- The working directory where it was started
- The model used
- All messages (user, assistant, tool results)
- A title (auto-generated or manually set)

Sessions are append-only — messages are never deleted, only compacted or branched away from.

## Switching Sessions

### Session Picker

Press `Ctrl+S` or type `/session` to open the session picker. Sessions are listed newest-first with their title, model, and message count. Filter by typing, navigate with arrow keys, select with Enter.

### Creating a New Session

```
/session new
```

This creates a new blank session linked to the current one. The new session starts empty but retains a reference to its parent for navigation.

### Resuming a Session

Select any session from the picker to switch to it. The conversation history is fully restored, including all messages and tool results.

## Branching

Branching lets you go back to an earlier point in the conversation and try a different approach — without losing the work you've already done.

### How to Branch

Type `/branch` to open the branch picker. It shows all entries on the current branch with their content preview. Select an entry to branch from that point.

After branching, the conversation rewinds to that entry. New messages continue from there, creating a new path in the tree. The old path is preserved and can be navigated back to.

### Branch Summaries

When you branch, piglet writes a `branch_summary` entry that captures what the abandoned branch was about. This context is visible in the tree view but doesn't pollute the active conversation.

### Example

```
Start → Assistant → "Try approach A" → Assistant (A didn't work)
                  ↘
                    "Try approach B" → Assistant (B works!) ← you are here
```

The LLM only sees: Start → Assistant → "Try approach B" → Assistant. The failed approach A is preserved in the tree but doesn't consume context.

## Forking

Forking creates a **new session file** that starts with a copy of the current conversation:

```
/fork
```

Use fork when you want to:

- Explore a tangent without affecting the main session
- Create project-specific continuations from a general setup session
- Preserve a checkpoint before a risky operation

The forked session records its parent session ID and how many messages were copied, so the relationship is traceable.

### Fork vs Branch

| | Branch | Fork |
|-|--------|------|
| **Creates** | New path in same file | New session file |
| **Use when** | Trying alternatives from same point | Diverging into separate work |
| **History** | Shared tree structure | Independent after fork point |
| **Navigation** | `/branch` picker | `/session` picker |

## Session Tree

View the tree structure of your sessions:

```
/tree
```

This shows the parent/child structure within the current session:

```
[01] user: "explain the auth flow"
  [02] assistant: "The auth flow starts with..."
    [03] user: "refactor it to use JWT"
      [04] assistant: "Here's the refactored version..."
    [05] (branch) tried JWT, switching to OAuth
      [06] user: "refactor it to use OAuth instead"  ← current
```

Entries on the active path are highlighted. Branch summaries and labels are shown inline.

## Compaction

Long conversations accumulate tokens. Compaction summarizes older messages to keep the context within the model's window.

### Auto-Compaction

When configured, piglet automatically compacts when input tokens exceed the threshold:

```yaml
# ~/.config/piglet/config.yaml
agent:
  compactAt: 100000        # Compact at 100k input tokens
  compactKeepRecent: 8     # Always keep 8 most recent messages
```

If an extension registers a compactor (the context pack provides an LLM-based one), it generates an intelligent summary. Otherwise, piglet uses a static fallback that keeps the first message plus the most recent ones.

### Manual Compaction

```
/compact
```

Manually trigger compaction at any time. Requires at least 4 messages.

### How It Works

Compaction writes a checkpoint entry to the session file. The checkpoint contains the summarized conversation up to that point. When the session is loaded, messages before the checkpoint are replaced by the summary. Messages after the checkpoint are preserved as-is.

Each branch compacts independently — different branches can have different compaction states.

## Labels

Labels are bookmarks you can attach to any entry in the session tree. They appear in the tree view and branch picker for easy navigation.

Labels are set via extensions (the session-tools extension provides this capability). Labels are last-write-wins per entry — setting a new label replaces the old one, and an empty label clears it.

## Searching

Search across all sessions:

```
/search query
```

This searches session titles and content for matches. Select a result to switch to that session.

## Auto-Titling

After the first exchange in a new session, piglet automatically generates a title based on the conversation content. This requires:

- The auto-title extension (part of pack-agent)
- `autoTitle: true` in config (the default)

To set a title manually:

```
/title My Custom Title
```

## Storage Format

Sessions are stored as line-delimited JSON (JSONL) files. Each line is an entry with a type, timestamp, ID, and parent ID forming a tree structure.

### Entry Types

| Type | Purpose |
|------|---------|
| `meta` | Session metadata (ID, CWD, model, title, timestamps) |
| `user` | User message |
| `assistant` | Assistant response |
| `tool_result` | Tool execution result |
| `compact` | Compaction checkpoint (contains summarized messages) |
| `branch_summary` | Branch point marker (captures abandoned path context) |
| `custom_message` | Extension-written message (appears in conversation on reload) |
| `label` | Bookmark label on a target entry |

### Example File

```jsonl
{"type":"meta","ts":"2026-04-01T10:00:00Z","data":{"id":"abc123","cwd":"/project","model":"claude-opus-4-6","createdAt":"2026-04-01T10:00:00Z"}}
{"type":"user","id":"a1b2c3d4","parentId":"","ts":"2026-04-01T10:00:01Z","data":{"content":"explain the auth flow"}}
{"type":"assistant","id":"e5f6a7b8","parentId":"a1b2c3d4","ts":"2026-04-01T10:00:05Z","data":{"content":[{"type":"text","text":"The auth flow..."}],"model":"claude-opus-4-6","usage":{"inputTokens":500,"outputTokens":200}}}
```

### Tree Structure

Entries link via `id` and `parentId` fields. The session tracks a leaf pointer — the most recent entry on the active branch. Building the conversation for the LLM means walking from the leaf to the root, collecting messages along the path.

Sessions created before tree support (without `id`/`parentId` fields) are transparently upgraded in memory with sequential IDs on load.

### File Location

```
~/.config/piglet/sessions/
├── abc123.jsonl
├── def456.jsonl
└── ...
```

Each file is a single session. Files are named by the session's UUID. The session directory can be overridden via `XDG_CONFIG_HOME`.
