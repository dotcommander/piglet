# Repomap

Generates a ranked, token-budgeted repository structure map with extracted symbols, injected into the system prompt to give the LLM a codebase overview.

## Capabilities

| Capability | Name | Description |
|------------|------|-------------|
| tool | `repomap_refresh` | Force rebuild the map after major file changes |
| tool | `repomap_show` | Show the current map without rebuilding |
| prompt | "Repository Map" | Ranked file + symbol listing |
| event | `repomap-stale-check` | Auto-rebuild when tracked files change (debounced 30s) |

## Prompt Order

15

## Pipeline

**Scan → Parse → Rank → Format**

1. **Scan**: Uses `git ls-files` (or filesystem walk). Filters by supported extensions, skips `vendor`/`node_modules`/etc., ignores files >50KB
2. **Parse**: Go files use `go/ast`; other languages use regex-based extraction for exports, classes, and functions. Runs in parallel
3. **Rank**: Scoring heuristics — entry points (+50/+30), exported symbols (+1 each), import references (+10 per importer), depth penalty
4. **Format**: Markdown output with token budget enforcement

## Supported Languages

Go, TypeScript/JavaScript, Python, Rust, C/C++, Java, Ruby, Lua, Zig, Swift, Kotlin.

## Configuration

File: `~/.config/piglet/config.yaml`

```yaml
repomap:
  maxTokens: 1024        # token budget (default)
  maxTokensNoCtx: 2048   # budget when no files in conversation
```

## Stale Detection

On `turn_end`, checks file modification times against the cached build. Rebuilds if files changed, with a 30-second debounce to avoid thrashing.
