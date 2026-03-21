# Ideas

Ranked feature ideas for piglet. Each section is a self-contained task list.

---

## 1. Persistent Project Memory (10/10)

Every new session starts blank — zero knowledge of past decisions, patterns, or files edited. A per-project memory store lets the agent write structured facts that are automatically injected into the system prompt on startup. Over time piglet genuinely learns your project.

### Design

Self-contained `memory/` package that registers itself entirely through `ext.App` — no changes to `tool/`, `prompt/`, or `core/`. Called from `cmd/piglet/main.go` like any extension.

### Tasks

- [ ] **Create `memory/` package** — new package with `Register(app *ext.App)` entry point. Contains storage, tools, command, and prompt section — all registered via `ext.RegisterTool`, `ext.RegisterCommand`, `ext.RegisterPromptSection`.
- [ ] **Storage layer** (`memory/store.go`) — JSONL file at `~/.config/piglet/memory/<sha256(cwd)[:12]>.jsonl`. Operations: `Load`, `Set(key, value, category)`, `Get(key)`, `List(category)`, `Delete(key)`. Atomic writes via temp+rename pattern (same as `tool/helpers.go:atomicWrite`).
- [ ] **Three tools** (`memory/tools.go`) — register via `app.RegisterTool`:
  - `memory_set(key, value, category?)` — upsert a fact
  - `memory_get(key)` — retrieve a single fact
  - `memory_list(category?)` — list all, optionally filtered
- [ ] **Prompt section** (`memory/prompt.go`) — `app.RegisterPromptSection` at order=50. Loads facts for current cwd, formats as "Project Memory" block. Cap at ~2000 tokens (~8000 chars); oldest facts truncated first.
- [ ] **`/memory` command** (`memory/command.go`) — `app.RegisterCommand`: no args = list all, `clear` = wipe, `delete <key>` = remove one.
- [ ] **Wire in `main.go`** — single call: `memory.Register(app)` after `ext.NewApp(cwd)`, before `prompt.Build()`.
- [ ] **Tests** — store round-trip (set/get/list/delete), prompt section cap, concurrent access.

---

## 2. Conversation Branching (9/10)

`session.Fork(keepMessages int)` exists at `session/session.go:207` but is never exposed. Branching turns linear chat into a decision tree — explore alternative approaches, then jump back.

### Design

Commands can't directly mutate TUI state — they write to `cmdResult` callbacks. Session swapping needs a new callback (`swapSession`) in the `cmdResult`/`ext.App` chain. The `/branch` command forks the session via the existing `session.Fork()`, then signals the TUI to swap.

### Tasks

- [ ] **Add branch metadata to `session.Meta`** — extend with `ParentID string` and `ForkPoint int` fields (`session/session.go:28`). Update `Fork()` to set `ParentID = parent.ID` and `ForkPoint = keepMessages`.
- [ ] **Add session swap callback to `ext.App`** — new `WithSwapSession` bind option and `SwapSession(sess *session.Session)` method (`ext/app.go`). Writes to `cmdResult.newSession` field.
- [ ] **Wire session swap in TUI** — in `tui/app.go:applyPendingResult()`, if `r.newSession != nil`: close old session, set `m.cfg.Session = newSession`, reload messages into agent.
- [ ] **Register `/branch` command** — in `command/builtins.go`. Needs session access: accept `sessDir` param (already passed to `RegisterBuiltins`). Forks current session via `ext.App.ConversationMessages()` + create new session + copy messages + call `SwapSession`.
- [ ] **Enhance `/session` picker** — show branched sessions with "forked from: <parent-id[:8]>" in the description. Read `ParentID` from `session.Summary`.
- [ ] **Add `ParentID` to `session.Summary`** — propagate from Meta through `scanSummary()`.
- [ ] **Tests** — fork preserves `ParentID`/`ForkPoint`, independent histories, session swap callback fires.

---

## 3. Background Agent (9/10)

`core.Agent.Start()` returns a channel and cancellation is context-based. Adding a second agent slot enables parallel human+machine work in one terminal — fire off test runs or analysis while continuing to chat.

### Design

Follows the extension pattern: `/bg` and `/bg-cancel` are registered commands via `ext.App`. The TUI owns the background agent lifecycle via `ext.App` callbacks (`RunBackground`, `CancelBackground`). Commands never touch `core.Agent` or TUI state directly.

### Tasks

- [ ] **Add background agent callbacks to `ext.App`** — `WithRunBackground(fn)` and `WithCancelBackground(fn)`. `RunBackground(prompt string) error` starts a background agent. `CancelBackground()` stops it. `IsBackgroundRunning() bool` for guard.
- [ ] **Add background state to `tui.Model`** — `bgAgent *core.Agent`, `bgEventCh <-chan core.Event`, `bgTask string`, `bgResult strings.Builder`. Wire the `ext.App` callbacks in `bindApp()`.
- [ ] **Register `/bg` command** — in `command/builtins.go`. Takes a prompt string, calls `a.RunBackground(prompt)`. Guards against concurrent runs via `a.IsBackgroundRunning()`.
- [ ] **Register `/bg-cancel` command** — calls `a.CancelBackground()`.
- [ ] **TUI background agent factory** — in the `RunBackground` callback, create a new `core.Agent` with same provider/model/system but read-only tools (read, grep, find, ls) and `MaxTurns: 5`. Start it, store channel.
- [ ] **Route background events** — in `tui/app.go`'s `Update`, poll `bgEventCh`. Accumulate `EventStreamDelta` text. On `EventAgentEnd`, inject result as a system message, clear background state.
- [ ] **Status bar indicator** — add `bgTask string` to `StatusBar`. Show "bg: <task>" when running.
- [ ] **Tests** — verify build, vet, full test suite.

---

## 4. Auto-Title + Semantic Session Search (7/10)

Sessions are currently listed by timestamp and ID only. LLM-generated titles after the first exchange plus a `/search` command would make session history navigable.

### Tasks

- [ ] **Generate title after first exchange** — in `tui/app.go`, after the first `EventAgentEnd`, fire a lightweight LLM call (same provider, short max_tokens ~50) with the first user message + assistant response asking for a 5-word title. Store via `session.SetTitle(title)`.
- [ ] **Add `Session.SetTitle(title string)`** — in `session/session.go`, write a "meta" entry updating the title. The `Meta.Title` field already exists.
- [ ] **Update session picker display** — in `command/builtins.go` (lines 194-234), show the title prominently in the picker label instead of just ID[:8].
- [ ] **Register `/search` command** — in `command/builtins.go`. Accepts a query string, loads all `session.Summary` via `session.List()`, does substring matching on `Title` + `CWD`. Shows results in the picker modal.
- [ ] **Optional: keyword index** — for larger session counts, maintain a `~/.config/piglet/sessions/index.jsonl` with `{id, title, keywords[]}`. Rebuild on `/search --reindex`.
- [ ] **Add `/title` command** — manually set or override the auto-generated title for the current session.

---

## 5. Diff-Aware Context Injection (6/10)

Auto-inject `git diff` summary into the system prompt on startup so the agent immediately knows what you've been working on. Eliminates the "here's what I changed" preamble.

### Tasks

- [ ] **Create `prompt/gitcontext.go`** — on startup, run `git diff --stat` and `git log --oneline -5` in the cwd. Capture output (bounded: 50 lines max).
- [ ] **Register as prompt section** — call `ext.RegisterPromptSection` with order=40 (before memory at 50). Title: "Recent Changes". Only inject if cwd is a git repo (check `.git/` existence).
- [ ] **Wire into bootstrap** — call registration from `cmd/piglet/main.go` after `ext.NewApp(cwd)` is created but before `prompt.Build()`.
- [ ] **Handle non-git gracefully** — if `git` is not found or cwd isn't a repo, skip silently. No error messages.
- [ ] **Cap output size** — truncate diff stat to first 30 files if larger. Show "and N more files" suffix.
- [ ] **Optional: changed file contents** — for files with <50 lines of diff, include the actual `git diff` hunks so the agent can see what changed, not just file names.
