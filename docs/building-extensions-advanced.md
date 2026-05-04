# Building Extensions: Advanced Patterns

This guide is for extension authors who have shipped working extensions and now want to tackle multi-extension coordination, interceptor chains, shared state, performance, and testing. It assumes you are comfortable with the basics from [Building Extensions](building-extensions.md).

## Table of Contents

- [Architecture](#architecture)
- [Coordination](#coordination)
- [Tool Execution Lifecycle](#tool-execution-lifecycle)
- [Performance](#performance)
- [Debugging](#debugging)
- [Testing](#testing)
- [Case Studies](#case-studies)
- [Reference](#reference)

---

## Architecture

### Three-Layer Split

Split extensions that grow beyond 200 lines into three layers. Each layer has a clear responsibility boundary:

```
my-extension/
├── register.go     # Wiring: sdk.Register, e.RegisterTool, e.OnInitAppend
├── engine.go       # I/O coordination: store access, LLM calls, file operations
├── classify.go     # Pure logic: no I/O, no SDK imports, no file access
└── defaults/       # Embedded default configs
    └── prompt.md
```

**Decision heuristic:**

| Layer | Contains | Imports | Test with |
|-------|----------|---------|-----------|
| `register.go` | SDK wiring, `OnInitAppend`, tool/command registration | `sdk`, `engine` | Integration tests |
| `engine.go` | State management, I/O coordination, store operations | `classify`, `os`, standard library | Unit tests with stubs |
| `classify.go` | Pure functions, data transformations | Standard library only | `go test` directly |

The pure layer is the seam that makes unit tests possible without mocking the SDK. Guard it: if `classify.go` ever imports `sdk`, `os`, or any I/O package, the split has failed.

**safeguard example:** `classify.go` contains `ClassifyCommand`, `ValidateInjection`, and `SplitSegments` — all pure string analysis. `engine.go` holds `BlockerWithConfig` and `AuditLogger`. `register.go` wires them into the SDK with a single `Register()` function.

### OnInit / OnInitAppend Pattern

Extensions often need the working directory or config directory before they can initialize. The SDK provides two hooks:

```go
func Register(e *sdk.Extension) {
    s := &myState{}

    // OnInitAppend — runs after the initialize handshake, when CWD is available.
    // Append (not replace) so multiple packages in a pack can each add their own.
    e.OnInitAppend(func(x *sdk.Extension) {
        start := time.Now()
        x.Log("debug", "[myext] OnInit start")

        store, err := NewStore(x.CWD())
        if err != nil {
            x.Log("debug", fmt.Sprintf("[myext] OnInit failed (%s)", time.Since(start)))
            return
        }
        s.store = store

        x.RegisterPromptSection(sdk.PromptSectionDef{
            Title:   "My Context",
            Content: BuildPrompt(s.store),
            Order:   50,
        })

        x.Log("debug", fmt.Sprintf("[myext] OnInit complete (%s)", time.Since(start)))
    })
}
```

**Rules:**

1. Always use `OnInitAppend`, never `OnInit` — `OnInit` replaces, `OnInitAppend` chains. In a pack binary with 10+ extensions, a replace would silently break others.
2. Store initialization results in a struct captured by closures — `s` in the example above. Tool `Execute` closures read `s.store` at call time, not at registration time.
3. Guard nil state in every `Execute`: `if s.store == nil { return sdk.ErrorResult("not available"), nil }`. Before `OnInit` completes, tools are registered but state is nil.

### Graceful Degradation

Extensions should fail gracefully when dependencies are missing. RTK demonstrates the canonical pattern:

```go
func Register(e *sdk.Extension) {
    rtkPath, err := exec.LookPath("rtk")
    if err != nil {
        // RTK not found — skip all registrations. The pack continues without it.
        return
    }
    // ... register interceptor and prompt section
}
```

Returning early from `Register` means the extension contributes zero registrations. The pack's `safety.Register` wrapper ensures a panic in any single extension doesn't crash the pack.

### Toggle Pattern

Expose runtime on/off toggles with a state struct and command wiring:

```go
// In engine.go:
type toggleState struct {
    enabled bool
}

func (t *toggleState) IsEnabled() bool {
    return t.enabled
}

// In register.go:
func Register(e *sdk.Extension) {
    toggle := &toggleState{enabled: true}

    e.RegisterCommand(sdk.CommandDef{
        Name:        "my-feature",
        Description: "Toggle my feature. Usage: /my-feature on|off|status",
        Handler: func(_ context.Context, args string) error {
            switch strings.TrimSpace(args) {
            case "on":
                toggle.enabled = true
                e.Notify("MyFeature: ON")
            case "off":
                toggle.enabled = false
                e.Notify("MyFeature: OFF")
            case "status":
                e.Notify(fmt.Sprintf("MyFeature: %v", toggle.enabled))
            }
            return nil
        },
    })

    e.RegisterInterceptor(sdk.InterceptorDef{
        Name:     "my-feature",
        Priority: 500,
        Before: func(_ context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
            if !toggle.IsEnabled() {
                return true, args, nil // skip when disabled
            }
            // ... gate logic
            return true, args, nil
        },
    })
}
```

Toggles are runtime-only. They survive within a session and reset on restart. For persistence across restarts, write to `config.yaml` via `e.ConfigGet`/`e.ConfigDir`.

---

## Coordination

### Interceptor Priority and Gate Chains

Interceptors run in priority order (higher number = runs first). This creates a chain where each interceptor can block or modify the call:

```
Priority 2000  safeguard.Before()    → block dangerous commands
Priority 1000  toolbreaker.Before()  → block circuit-broken tools
Priority  900  toolbreaker.Before()  → (separate instance in pack-agent)
Priority  500  my-transform.Before() → rewrite arguments
Priority  100  rtk.Before()          → rewrite bash commands for token savings
```

**Design rules:**

| Priority Range | Use Case | Examples |
|----------------|----------|----------|
| 2000+ | Security — block before anything else runs | safeguard, preflight |
| 900–1999 | Circuit breakers — block after safety clears | toolbreaker |
| 500–899 | Transformation — modify arguments | custom rewriters |
| 0–499 | Default — logging, enrichment | audit, metrics |

Each interceptor receives the args as modified by the previous one. If any interceptor returns `(false, _, nil)`, execution stops — the tool is never called and a synthetic result is returned.

### Interceptor + Event Handler Coordination

The circuit breaker pattern shows how an interceptor and event handler share state:

```go
// breakerState is shared between the interceptor (Before gate)
// and the event handler (failure tracking).
type breakerState struct {
    mu       sync.Mutex
    failures map[string]int       // tool name → consecutive failure count
    disabled map[string]time.Time // tool name → cooldown expiry
}

// Interceptor blocks calls to disabled tools
e.RegisterInterceptor(sdk.InterceptorDef{
    Name:     "tool-circuit-breaker",
    Priority: 1000,
    Before: func(_ context.Context, toolName string, _ map[string]any) (bool, map[string]any, error) {
        if state.isDisabled(toolName) {
            return false, nil, nil  // block — returns synthetic result
        }
        return true, nil, nil
    },
})

// Event handler tracks failures from EventToolEnd
e.RegisterEventHandler(sdk.EventHandlerDef{
    Name:   "tool-circuit-breaker",
    Events: []string{"EventToolEnd"},
    Handle: func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
        var evt struct {
            ToolName string `json:"ToolName"`
            IsError  bool   `json:"IsError"`
        }
        json.Unmarshal(data, &evt)
        state.record(evt.ToolName, evt.IsError)
        return nil
    },
})
```

**Critical subtlety:** When the interceptor blocks a call, `EventToolEnd` fires with `IsError: false` (it's a synthetic block, not an execution error). The event handler must skip tracking for disabled tools, or it would falsely reset the breaker on every block:

```go
// Skip tools that are already disabled — their EventToolEnd comes
// from the interceptor block (IsError=false), not actual execution.
if tracker.IsDisabled(evt.ToolName, limit) {
    return nil
}
```

### Event Handler Ordering

Event handlers with the same event filter run in registration order (within an extension). Use priority to control cross-extension ordering:

```go
// Context reset runs first (priority 10) — clears stale facts
e.RegisterEventHandler(sdk.EventHandlerDef{
    Name:     "memory-context-reset",
    Priority: 10,
    Events:   []string{"EventAgentStart"},
    Handle:   func(/* ... */) { s.store.Clear("_context") },
})

// Extractor runs second (priority 50) — writes new facts
e.RegisterEventHandler(sdk.EventHandlerDef{
    Name:     "memory-extractor",
    Priority: 50,
    Events:   []string{"EventTurnEnd"},
    Handle:   func(/* ... */) { s.extractor.Extract(data) },
})

// Clearer runs third (priority 60) — trims old tool results after extractor
registerClearer(e)
```

### Inter-Extension Communication

Extensions within the same pack communicate through shared Go state (same binary). Extensions across packs use the pub/sub event bus:

```go
// Pack A — publisher
e.RegisterEventHandler(sdk.EventHandlerDef{
    Name:   "my-cleanup",
    Events: []string{"EventAgentEnd"},
    Handle: func(_ context.Context, _ string, _ json.RawMessage) *sdk.Action {
        e.Publish("myext:cleanup-done", map[string]any{"session": sessionID})
        return nil
    },
})

// Pack B — subscriber
e.Subscribe(ctx, "myext:cleanup-done", func(data any) {
    // React to the event
})
```

### Deferred Tool Registration

Set `Deferred: true` on tools that the LLM needs to know about but rarely calls. Deferred tools send only name and description to the model API; the full parameter schema is fetched on demand. This reduces prompt token consumption for large tool inventories:

```go
e.RegisterTool(sdk.ToolDef{
    Name:        "memory_related",
    Description: "Find all facts related to a key by traversing memory graph edges.",
    Deferred:    true,  // schema sent only when the LLM decides to call
    Parameters:  /* ... */,
})
```

---

## Tool Execution Lifecycle

### Full Sequence

```
LLM: "use tool foo with args {x: 1}"
  │
  ├─► EventToolStart     { ToolCallID, ToolName, Args }
  │
  ├─► Interceptor.Before  (priority order: 2000 → 1000 → 500 → 100)
  │     ├─ allow=false → synthetic result, skip execute()
  │     └─ allow=true  → pass (possibly modified) args to execute()
  │
  ├─► [execute() runs]
  │     ├─► EventToolUpdate   (if onUpdate is called — not yet in SDK)
  │     └─► returns ToolResult { Content, IsError, ErrorCode?, ErrorHint? }
  │
  ├─► Interceptor.After   (reverse priority order)
  │     └─ can modify result before it enters context
  │
  ├─► EventToolEnd       { ToolCallID, ToolName, Result, IsError }
  │
  └─► ToolResult committed to session in assistant source order
```

Tools from the same assistant turn run in parallel by default. The interceptor chain serializes only the gate check — actual tool execution is concurrent.

### Error Handling with ToolErr

Use `sdk.ToolErr` for errors the LLM should pattern-match for recovery. It produces a canonical `[error:CODE]` prefix that the LLM reads to decide what to do:

```go
// Machine-readable error — LLM sees the code and can adapt
return sdk.ToolErr(
    sdk.ToolErrFileNotFound,
    fmt.Sprintf("file not found: %s", path),
    "use read tool to verify the path exists",
), nil

// Simple error — LLM sees only the text
return sdk.ErrorResult("something went wrong"), nil

// Success
return sdk.TextResult("operation completed"), nil
```

**Standard error codes:**

| Code | When to Use |
|------|-------------|
| `INVALID_ARGS` | Malformed or missing arguments |
| `FILE_NOT_FOUND` | File doesn't exist |
| `FILE_STALE` | File changed between read and edit |
| `FILE_TOO_LARGE` | File exceeds size limit |
| `NOT_REGULAR_FILE` | Path is a directory, symlink, etc. |
| `PERMISSION_DENIED` | Insufficient permissions |
| `NOT_UNIQUE` | Match found multiple targets |
| `TIMEOUT` | Operation exceeded time limit |
| `EXIT_NONZERO` | Shell command exited non-zero |
| `IO_ERROR` | General I/O failure |
| `INTERNAL` | Unexpected internal error |

The `hint` parameter is critical — it tells the LLM what to try next. Without it, the model can only guess at recovery.

### Guarding nil State in Execute

Tools registered before `OnInit` completes have nil state. Every `Execute` function must handle this:

```go
Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
    if s.store == nil {
        return sdk.ErrorResult("memory store not available"), nil
    }
    // ... normal execution
},
```

This is not defensive programming — it's a structural requirement. Between `Register()` and `OnInit`, the tool is live but uninitialized.

### The Execute Return Contract

Every `Execute` must return exactly one of:

```go
// Success
return sdk.TextResult("operation done"), nil

// Error (simple)
return sdk.ErrorResult("bad input"), nil

// Error (coded — for LLM recovery)
return sdk.ToolErr(sdk.ToolErrFileNotFound, "not found", "check path"), nil

// Never return a non-nil error as the second value — that's an SDK transport error,
// not a tool error. Tool errors go in the ToolResult, not the Go error.
```

**Never** return `(nil, err)` for expected failures. Return `(sdk.ErrorResult(...), nil)`. The Go error channel is for transport failures (JSON-RPC disconnected, context cancelled), not business logic.

---

## Performance

### Atomic Pointer for Hot-Path Interceptors

Interceptors run on every tool call. If initialization is async (in `OnInitAppend`), use `sync/atomic` so the Before hook never races with initialization:

```go
var blocker atomic.Pointer[func(context.Context, string, map[string]any) (bool, map[string]any, string)]

e.OnInitAppend(func(e *sdk.Extension) {
    fn := BlockerWithConfig(cfg, compiled, e.CWD(), audit)
    blocker.Store(&fn)
})

e.RegisterInterceptor(sdk.InterceptorDef{
    Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
        if fn := blocker.Load(); fn != nil {
            allow, modified, reason := (*fn)(ctx, toolName, args)
            return allow, modified, nil
        }
        return true, args, nil // allow-all before OnInit completes
    },
})
```

Before `OnInit` completes, `blocker.Load()` returns nil and the interceptor falls through to allow-all. This is the correct default — fail-open during initialization.

### Skip Early in Hot Paths

Optimize interceptor Before hooks by checking the tool name first. RTK only cares about bash commands:

```go
Before: func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
    if toolName != "bash" {
        return true, args, nil  // skip all non-bash tools immediately
    }
    // ... expensive logic only for bash
},
```

For sets of tool names, use a `map[string]struct{}` lookup:

```go
readOnly := map[string]struct{}{
    "read": {}, "grep": {}, "find": {}, "ls": {},
}

Before: func(_ context.Context, toolName string, _ map[string]any) (bool, map[string]any, error) {
    if _, ok := readOnly[toolName]; ok {
        return true, nil, nil  // skip read-only tools
    }
    // ... check write tools
},
```

### Embedded Defaults for Config Files

Use `//go:embed` for default configuration that users can override. The `xdg` package provides `LoadOrCreateExt` for safe config file management:

```go
import (
    _ "embed"
    "github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed defaults/prompt.md
var defaultPrompt string

func Register(e *sdk.Extension) {
    e.OnInitAppend(func(x *sdk.Extension) {
        content := xdg.LoadOrCreateExt("myext", "prompt.md", strings.TrimSpace(defaultPrompt))
        x.RegisterPromptSection(sdk.PromptSectionDef{
            Title:   "My Extension",
            Content: content,
            Order:   50,
        })
    })
}
```

`LoadOrCreateExt` resolves in this order:
1. `~/.config/piglet/extensions/myext/prompt.md` (new location)
2. `~/.config/piglet/prompt.md` (old flat location, migrated)
3. Embedded default (written to new location)

Users can customize behavior by editing the file at location 1.

### Atomic File Writes

Use `xdg.WriteFileAtomic` for any write that replaces existing content:

```go
xdg.WriteFileAtomic(path, []byte(content+"\n"))
```

This writes to a temp file, fsyncs, then renames — preventing partial writes on crash.

---

## Debugging

### Structured Logging Pattern

Gate diagnostic output behind debug level and use structured fields:

```go
e.OnInitAppend(func(x *sdk.Extension) {
    start := time.Now()
    x.Log("debug", "[myext] OnInit start")

    // ... initialization work ...

    x.Log("debug", fmt.Sprintf("[myext] OnInit complete (%s)", time.Since(start)))
})
```

View with `piglet --debug`. All `e.Log("debug", ...)` calls write to the host's debug log.

### Audit Logging for Security Extensions

Write tool decisions to a JSONL audit log for post-hoc analysis:

```go
type AuditLogger struct {
    mu   sync.Mutex
    file *os.File
}

func (a *AuditLogger) Log(tool, decision, reason, detail string) {
    a.mu.Lock()
    defer a.mu.Unlock()
    entry := map[string]any{
        "ts":        time.Now().Format(time.RFC3339),
        "tool":      tool,
        "decision":  decision,
        "reason":    reason,
        "detail":    detail,
    }
    json.NewEncoder(a.file).Encode(entry)
}
```

The audit log lives in the extension's config directory. Query with:

```bash
cat ~/.config/piglet/extensions/safeguard/audit.jsonl | jq '.decision == "block"'
```

### Graceful Missing Binary

Warn on startup if a required external tool is missing, then skip silently:

```go
func Register(e *sdk.Extension) {
    rtkPath, err := exec.LookPath("rtk")
    if err != nil {
        return // not found — skip all registrations
    }
    // ... register interceptor
}
```

For tools where absence should warn the user:

```go
e.OnInitAppend(func(x *sdk.Extension) {
    if _, err := exec.LookPath("gopls"); err != nil {
        x.NotifyWarn("gopls not found — LSP features disabled for Go")
        return
    }
    // ... register LSP tools
})
```

---

## Testing

### Pure Function Unit Tests

Functions in `classify.go` (or any pure-logic file) test directly with no SDK mocks:

```go
// classify_test.go
func TestClassifyCommand_ReadOnly(t *testing.T) {
    tests := []struct{ cmd, want string }{
        {"ls -la", CommandReadOnly},
        {"cat file.txt", CommandReadOnly},
        {"grep pattern *.go", CommandReadOnly},
        {"rm file.txt", CommandWrite},
    }
    for _, tt := range tests {
        got := ClassifyCommand(tt.cmd)
        if got != tt.want {
            t.Errorf("ClassifyCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
        }
    }
}
```

No SDK imports, no mocks, no test harness — just standard Go tests.

### Extracting Testable Functions from Closures

The circuit breaker extracts its logic into a state struct with exported methods, then wraps those in closures for the SDK:

```go
// breaker.go — testable in isolation
func NewBreakerFuncs(maxFails int, cooldown time.Duration) BreakerFuncs {
    state := newBreakerState(maxFails, cooldown)
    return BreakerFuncs{
        Before:  func(_ context.Context, toolName string, _ map[string]any) (bool, map[string]any, error) {
            return !state.isDisabled(toolName), nil, nil
        },
        Handle: func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
            // ... parse event, record failure/success
            return nil
        },
    }
}

// breaker_test.go — tests the closures directly
func TestBreaker_BlocksAfterConsecutiveFailures(t *testing.T) {
    fns := NewBreakerFuncs(3, time.Minute)
    // ... simulate failures, verify block
}
```

This pattern tests the actual closures that the SDK calls, without needing a running piglet instance.

### Store Contract Testing

Test the data layer with round-trip verification:

```go
func TestStoreRoundTrip(t *testing.T) {
    dir := t.TempDir()
    store := NewTestStore(dir)

    store.Set("key1", "value1", "cat1")
    store.Set("key2", "value2", "cat1")

    facts := store.List("cat1")
    if len(facts) != 2 {
        t.Fatalf("expected 2 facts, got %d", len(facts))
    }

    fact, ok := store.Get("key1")
    if !ok || fact.Value != "value1" {
        t.Fatalf("expected value1, got %v", fact.Value)
    }
}
```

### Host Callback Stub Pattern

When testing code that calls host methods (`e.Chat`, `e.AuthGetKey`, etc.), accept the host dependency as an interface:

```go
// engine.go
type LLMCaller interface {
    Chat(ctx context.Context, req sdk.AgentRequest) (*sdk.AgentResponse, error)
}

func ProcessWithLLM(ctx context.Context, caller LLMCaller, input string) (string, error) {
    resp, err := caller.Chat(ctx, sdk.AgentRequest{
        Messages: []sdk.ChatMessage{{Role: "user", Content: input}},
    })
    if err != nil {
        return "", err
    }
    return resp.Content, nil
}

// engine_test.go
type stubCaller struct {
    response string
    err      error
}

func (s *stubCaller) Chat(_ context.Context, _ sdk.AgentRequest) (*sdk.AgentResponse, error) {
    return &sdk.AgentResponse{Content: s.response}, s.err
}
```

---

## Case Studies

### pack-agent: Safeguard (Interceptor Chain + Audit)

**Problem:** Block dangerous bash commands before execution, with configurable profiles (strict/balanced/off) and audit logging.

**Architecture:**

```
safeguard/
├── register.go       # SDK wiring — single Register() function
├── safeguard.go      # BlockerWithConfig — the interceptor logic
├── classify.go       # ClassifyCommand — pure command analysis
├── classify_db.go    # Known-command database
├── injection.go      # ValidateInjection — shell injection detection
├── preflight.go      # RegisterPreflight — git state checks
├── breaker.go        # RegisterBreaker — circuit breaker (shared state pattern)
├── audit.go          # AuditLogger — JSONL decision log
├── config.go         # Config loading with profile constants
├── defaults/         # Embedded defaults
│   └── prompt.md
└── *_test.go         # 10+ test files, all test pure functions
```

**Key patterns:**

1. **Atomic pointer for async init** — `atomic.Pointer[func(...)]` stores the blocker closure after `OnInitAppend`. Before init completes, the interceptor allows all calls (fail-open).
2. **Priority 2000** — runs before everything else. No other interceptor sees a dangerous call because safeguard blocks it first.
3. **Pure classification** — `ClassifyCommand` has zero I/O imports. 15+ test cases cover edge cases (pipes, redirects, Docker, command substitution, brace expansion).
4. **Structured audit log** — every allow/block decision writes to JSONL. Post-hoc analysis without TUI access.
5. **Graceful off** — when `ProfileOff` is set, `OnInitAppend` doesn't store a blocker function. `blocker.Load()` returns nil and the Before hook falls through to allow-all.

### pack-context: Memory (Inject + React + Observe)

**Problem:** Persist project-specific facts across sessions, inject a compact index into the prompt, and extract context from tool results automatically.

**Architecture:**

```
memory/
├── register.go           # SDK wiring — 5 tools, 1 command, 3 event handlers, 1 prompt section
├── store.go              # JSONL-backed key/value store with relations
├── extractor.go          # Deterministic fact extraction from EventTurnEnd
├── prompt.go             # BuildMemoryPrompt — index generation
├── reinject.go           # Critical context survival across compaction
├── graph.go              # Related — graph traversal for memory relations
├── overflow.go           # Persist large tool results to disk
├── clearer.go            # Micro-compact old tool results
├── compact/              # Sub-package for compaction logic
│   ├── handler.go        # Compactor SDK handler with cooldown
│   ├── summarize.go      # LLM-based fact summarization
│   ├── strategies.go     # LightTrim, Microcompact strategies
│   └── helpers.go        # XML tag parsing for file lists
└── *_test.go             # 8+ test files
```

**Key patterns:**

1. **Progressive disclosure** — prompt section injects only an index (~8000 chars cap). Full facts are loaded on demand via `memory_get`. This keeps the prompt small while preserving access to all stored knowledge.
2. **State struct captured by closures** — `memoryState` holds `store` and `extractor`. All tool Execute closures reference `s.store`. The struct is nil until `OnInitAppend` completes.
3. **Event handler ordering** — `EventAgentStart` (priority 10) clears context facts, `EventTurnEnd` extractor (priority 50) writes new facts, clearer (priority 60) trims old results. The ordering guarantees reset happens before extraction.
4. **Compaction sub-package** — `compact/` is a self-contained module with its own `Storer` interface. The parent `register.go` creates a `compactAdapter` that bridges `memory.Store` to `compact.Storer`. This keeps compaction logic testable without the full memory store.
5. **Reinject across compaction** — `GatherCriticalContext` selects facts that should survive compaction (errors, plans, recent edits). `BuildReinjectMessage` formats them as a user message injected after compaction completes.

### pack-agent: Subagent (Tool → Host Process)

**Problem:** Spawn a full piglet instance in a tmux pane for parallel work, with timeout management, deduplication, and environment filtering.

**Key patterns:**

1. **Dedup cache** — identical task prompts return cached results instead of re-spawning agents. `normalizePrompt` strips whitespace for near-exact matching.
2. **Environment filtering** — `safeexec.FilterEnv` strips secrets from the child environment, then selectively adds back `TMUX`, `TMUX_TMPDIR`, and `*_API_KEY` vars.
3. **Dual timeout** — absolute wall-clock timeout kills regardless, inactivity timeout kills when the pane goes silent. Both are configurable via tool arguments.
4. **Coded errors** — timeout returns `sdk.ToolErr(sdk.ToolErrTimeout, ...)` with a hint to increase the timeout. The LLM can retry with adjusted parameters.
5. **Context cancellation** — the polling loop respects `ctx.Done()`. If the user cancels the parent turn, the subagent poll exits immediately (though the tmux pane continues running independently).

### pack-code: LSP (Intercept + React + Observe)

**Problem:** Bridge language server protocol into piglet tools, with prompt injection for LSP awareness.

**Key patterns:**

1. **Lazy initialization** — `Manager` is created in `OnInitAppend` when CWD is available. Tools check `mgr` for nil before use.
2. **Shared parameter schema** — `positionParams` is defined once and reused across `lsp_definition`, `lsp_references`, `lsp_hover`, and `lsp_rename`.
3. **Embedded prompt** — `//go:embed defaults/prompt.md` with `xdg.LoadOrCreateExt` for user customization.
4. **Deferred tools** — all LSP tools use `Deferred: true` to avoid sending large parameter schemas unless the LLM actually calls them.

### pack-agent: RTK (Interceptor + Prompt Section)

**Problem:** Rewrite bash commands to reduce token output, with a binary dependency that may not be installed.

**Key patterns:**

1. **Early return on missing binary** — `exec.LookPath("rtk")` fails gracefully. No registrations, no errors, no warnings.
2. **Tool-specific interceptor** — `if toolName != "bash" { return true, args, nil }` skips all non-bash calls instantly.
3. **Non-mutating pass-through** — if the rewrite fails or produces no change, return `true, args, nil` (original args untouched).
4. **Clone before modify** — `maps.Clone(args)` before mutating, so other interceptors in the chain see a clean map.
5. **Priority 100** — runs late in the chain, after security checks. Only rewrites commands that already passed safeguard.

---

## Reference

### Quick Decision Matrix

| Situation | Pattern | Section |
|-----------|---------|---------|
| Extension >200 LOC | Three-layer split (register/engine/classify) | [Architecture](#architecture) |
| Need CWD in initialization | OnInitAppend + atomic pointer | [Architecture](#architecture) |
| Optional dependency | Graceful degradation (early return from Register) | [Architecture](#architecture) |
| Runtime toggle | Toggle pattern (state struct + command) | [Architecture](#architecture) |
| Block dangerous tools | Interceptor with Priority 2000+ | [Coordination](#coordination) |
| Track tool failures | Interceptor + Event handler sharing state | [Coordination](#coordination) |
| Order event handlers | Priority field (lower = earlier) | [Coordination](#coordination) |
| Cross-pack communication | e.Publish / e.Subscribe | [Coordination](#coordination) |
| Reduce prompt tokens | Deferred: true on tools | [Coordination](#coordination) |
| LLM-recoverable errors | sdk.ToolErr with code + hint | [Tool Execution Lifecycle](#tool-execution-lifecycle) |
| Nil state before init | Guard nil in every Execute closure | [Tool Execution Lifecycle](#tool-execution-lifecycle) |
| Fast interceptor | Check tool name first, skip early | [Performance](#performance) |
| Async init + interceptor | sync/atomic for shared closure | [Performance](#performance) |
| User-customizable config | go:embed + xdg.LoadOrCreateExt | [Performance](#performance) |
| Debugging init timing | e.Log("debug", ...) with duration | [Debugging](#debugging) |
| Security audit trail | JSONL audit log | [Debugging](#debugging) |
| Test pure logic | Extract to classify.go, standard go test | [Testing](#testing) |
| Test interceptor logic | Extract closures via factory function | [Testing](#testing) |
| Test host callbacks | Accept LLMCaller as interface, stub in tests | [Testing](#testing) |

### Pack Safety Wrapper

Every official pack uses `safety.Register` to wrap each extension's `Register` call with panic recovery:

```go
// From extensions/packs/internal/safety/safety.go
func Register(e *sdk.Extension, name string, fn func(e *sdk.Extension)) {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("extension register panicked", "name", name, "panic", r)
        }
    }()
    fn(e)
}
```

One extension's panic does not crash the pack. Other extensions load normally, and the failed extension's capabilities are simply absent.

### Shared Internal Packages

Extensions share utilities through `extensions/internal/`:

| Package | Purpose | Consumers |
|---------|---------|-----------|
| `internal/xdg` | Config directory resolution, atomic file writes, load-or-create patterns | 30+ extensions |
| `internal/safeexec` | Environment filtering for subprocess spawning, allowlisted env vars | subagent, bulk |
| `internal/toolresult` | Tool result parsing helpers for interceptors | memory, sift, fossil |
| `internal/unicodeaudit` | Unicode character validation | skill |

**Gating rule:** An abstraction goes in `internal/` only when three or more existing extensions need it. Aspirational consumers don't qualify.

### Tool Parameter Type Conversion

JSON-RPC deserializes numeric arguments as `float64`, not `int`. Always convert:

```go
maxDepth := 3
if md, ok := args["max_depth"].(float64); ok && int(md) > 0 {
    maxDepth = int(md)
}
```

String arguments use type assertion with zero-value fallback:

```go
name, _ := args["name"].(string)  // empty string if wrong type or missing
if name == "" {
    return sdk.ErrorResult("name is required"), nil
}
```
