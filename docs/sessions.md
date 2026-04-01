# Session Architecture

## Design

Piglet sessions use tree-structured JSONL files. Each session is a single `.jsonl` file in `~/.config/piglet/sessions/`. Entries form a tree via `ID`/`ParentID` linking, enabling in-place branching within a single file. Fork creates a new file for project-specific continuations.

### Prior Art

This design draws from [pi-mono's session format](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/session.md), which pioneered tree-structured JSONL sessions for coding agents. Pi-mono uses per-entry `id`/`parentId` linking within a single file for in-place branching. Piglet adopts the same data model.

### Why Tree-Structured Sessions

Linear session history breaks down in real coding workflows:

1. **Exploratory branching** — "Try approach A, then try approach B from the same starting point." Without trees, the user must manually create a new session and re-establish context.

2. **Branch-and-resume** — Move the leaf to an earlier entry, leave a `branch_summary` capturing the abandoned path. The LLM sees the active branch without the dead-end conversation polluting context.

3. **Fork-and-specialize** — Start a general session, then fork into project-specific continuations. The parent session's context is preserved, and each fork carries only the divergent work.

4. **Compaction within branches** — Each branch compacts independently. A compaction entry replaces all ancestor messages on the branch path, so different branches can have different compaction states.

5. **Session navigation** — `/session tree` renders the parent/child structure across files. `/branch` shows the in-session tree for branching to earlier points.

### Piglet vs Pi-mono

| Capability | pi-mono | piglet |
|-----------|---------|--------|
| Storage | Single file, per-entry `id`/`parentId` | Single file, per-entry `ID`/`ParentID` |
| In-place branching | Yes | Yes |
| Branch summaries | `BranchSummaryEntry` with LLM-generated summaries | `branch_summary` entries with summary text |
| Context building | Walk `parentId` chain from leaf to root | Walk `ParentID` chain from leaf to root |
| Compaction | `CompactionEntry` with `firstKeptEntryId` | `compact` entry resets branch messages |
| Fork to new file | `parentSession` in header | `ParentID` + `ForkPoint` in meta |
| Entry types | 10+ (message, model change, label, custom, etc.) | 8 (meta, user, assistant, tool_result, compact, branch_summary, custom_message, label) |
| Legacy migration | Version field, auto-upgrade | Deterministic ID assignment on load |

Piglet supports custom_message and label entries for extension-driven annotations and bookmarks, while keeping the tree mechanics unchanged.

## File Format

Each line is a JSON object. Entries (except `meta`) have `id` and `parentId` fields forming the tree.

```jsonl
{"type":"meta","ts":"...","data":{"id":"uuid","cwd":"/path","createdAt":"..."}}
{"type":"user","id":"a1b2c3d4","parentId":"","ts":"...","data":{"content":"hello"}}
{"type":"assistant","id":"e5f6a7b8","parentId":"a1b2c3d4","ts":"...","data":{...}}
{"type":"user","id":"c9d0e1f2","parentId":"e5f6a7b8","ts":"...","data":{"content":"try X"}}
```

### Branching Example

```jsonl
{"type":"user","id":"01","parentId":"","ts":"...","data":{"content":"start"}}
{"type":"assistant","id":"02","parentId":"01","ts":"...","data":{...}}
{"type":"user","id":"03","parentId":"02","ts":"...","data":{"content":"approach A"}}
{"type":"assistant","id":"04","parentId":"03","ts":"...","data":{...}}
{"type":"branch_summary","id":"05","parentId":"02","ts":"...","data":{"summary":"tried A, didn't work","fromId":"04"}}
{"type":"user","id":"06","parentId":"05","ts":"...","data":{"content":"approach B"}}
```

Tree structure:
```
[01: start] ─── [02: assistant] ─┬─ [03: approach A] ─── [04: assistant]
                                  │
                                  └─ [05: summary] ─── [06: approach B] ← leaf
```

Context at leaf `06`: `[01, 02, 06]` — the branch summary is metadata, not a conversation message.

### Entry Types

| Type | Tree participant | Purpose |
|------|:---:|---------|
| `meta` | No | Session metadata (ID, CWD, model, title, parent, fork point). Rewritten on updates. |
| `user` | Yes | User message |
| `assistant` | Yes | Assistant response with content blocks |
| `tool_result` | Yes | Tool execution result |
| `compact` | Yes | Compaction checkpoint — contains a JSON array of replacement entries. On context build, resets all ancestor messages. |
| `branch_summary` | Yes | Marks a branch point. Contains summary text and the `fromId` of the abandoned leaf. Not included in conversation messages. |
| `custom_message` | Yes | Extension-written message that persists AND appears in `Messages()` on reload. Data: `{role, content}`. |
| `label` | Yes | Bookmark label on a target entry. Data: `{targetId, label}`. Empty label = clear. Last-write-wins. |

### Legacy Compatibility

Sessions created before tree support (entries without `id`/`parentId` fields) are transparently upgraded in memory on load. Deterministic sequential IDs (`L0`, `L1`, ...) are assigned, and each entry is chained linearly. New entries written to the session will have proper random IDs and chain from the legacy entries.

### Fork Metadata

When a session is forked to a new file, the meta entry includes:

```json
{"parentId": "parent-session-uuid", "forkPoint": 5}
```

- `parentId` — UUID of the source session
- `forkPoint` — number of messages copied from the parent (0 for a blank fork via `/session new`)

### Compaction

`AppendCompact` writes a `compact` entry whose `data` is a JSON array of sub-entries. During context building (leaf-to-root walk), encountering a compact entry resets the message list to the compact's content. Messages after the compact on the branch path are appended normally.

Multiple compactions are valid; each one on the active branch replaces everything before it.

## Commands

| Command | Effect |
|---------|--------|
| `/session` | Open session picker (tree-indented) |
| `/session new` | Fork with 0 messages — blank branch linked to current session |
| `/session tree` | ASCII tree display of all sessions with parent/child relationships |
| `/branch` | Picker showing current branch entries — select one to branch in-place |
| `/fork` | Fork current session to a new file (copies all messages) |

## API

### Session

| Method | Purpose |
|--------|---------|
| `New(dir, cwd)` | Create new session |
| `Open(path)` | Load existing session from JSONL |
| `Append(msg)` | Add message at current leaf |
| `AppendCompact(msgs)` | Write compaction checkpoint at current leaf |
| `Branch(entryID)` | Move leaf to earlier entry (in-place branching) |
| `BranchWithSummary(entryID, summary)` | Move leaf and write branch_summary entry |
| `Fork(keepMessages)` | Create new session file linked to this one |
| `Messages()` | Build message list by walking current branch (leaf to root) |
| `LeafID()` | Current leaf entry ID |
| `EntryInfos()` | Entry info for current branch (for display/picker) |
| `SetTitle(title)` | Update session title |
| `SetModel(model)` | Update model in metadata |
| `AppendCustomMessage(role, content)` | Write a message that persists and appears in Messages() on reload |
| `AppendLabel(targetID, label)` | Set or clear a bookmark label on an entry |
| `Label(entryID)` | Get the label for an entry |
| `FullTree()` | Full DAG view of all entries for tree rendering |
| `List(dir)` | List all sessions, newest first |
| `Close()` | Close session file |

### ext.App (extension-facing)

| Method | Purpose |
|--------|---------|
| `BranchSession(entryID)` | In-place branch, refreshes agent + TUI |
| `BranchSessionWithSummary(entryID, summary)` | Branch with summary, refreshes agent + TUI |
| `SessionEntryInfos()` | Entry info for current branch |
| `ForkSession()` | Fork to new file |
| `LoadSession(path)` | Switch to different session |
| `Sessions()` | List all sessions |
| `SessionFullTree()` | Full DAG for tree rendering |
| `AppendCustomMessage(role, content)` | Write persistent message to session |
| `SetSessionLabel(targetID, label)` | Set or clear bookmark label |
| `AppendSessionEntry(kind, data)` | Write custom extension entry |
