# Memory

Persistent per-project key-value memory that captures session context and facts across conversations.

## Capabilities

| Capability | Name | Description |
|------------|------|-------------|
| tool | `memory_set` | Save a key-value fact with optional category |
| tool | `memory_get` | Retrieve a fact by key |
| tool | `memory_list` | List all facts, optionally filtered by category |
| command | `/memory` | List, clear, or delete memories |
| prompt | "Project Memory" | Injects current facts into system prompt |

## Prompt Order

50

## Command Usage

```
/memory              # list all memories
/memory clear        # delete all memories
/memory clear context  # delete only session context
/memory delete <key> # delete a specific fact
```

## Storage

- Per-project JSONL file at `~/.config/piglet/memory/<cwd-hash>.jsonl`
- CWD is SHA256-hashed (first 12 chars) for project isolation
- Thread-safe with RWMutex; atomic writes via temp file + rename

## Automatic Context Extraction

On `EventTurnEnd`, the extension parses tool results (Read, Grep, Glob, Edit, Bash) and extracts:

- File paths → `ctx:file:*`
- Edit locations → `ctx:edit:*`
- Error messages → `ctx:error:*`
- Commands run → `ctx:cmd:*`

Keeps only the 50 most recent context facts (auto-prunes oldest).

## Compaction

When message count exceeds a threshold, context facts are compacted:

1. Groups facts by type (files, edits, errors, commands)
2. Optionally calls the LLM to produce a concise 5-10 line summary
3. Replaces granular facts with a single `ctx:summary`

## Prompt Limits

User facts capped at 8000 chars with 500 chars reserved for the context section.
