# Bulk

Runs shell commands across many items (git repos, directories, files, or explicit paths) in parallel with filtering and dry-run protection.

## Capabilities

| Capability | Name |
|------------|------|
| tool | `bulk` |

## Tool Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `source` | enum | Discovery mode: `git_repos`, `dirs`, `files`, `list` |
| `root` | string | Root directory to scan |
| `pattern` | string | Glob pattern (for `dirs`/`files`); supports `**` recursive matching |
| `list` | string[] | Explicit paths (for `list` source) |
| `filter` | string | Git condition (`dirty`, `clean`, `ahead`, `behind`, `diverged`) or shell predicate |
| `command` | string | Shell command to run per item |
| `concurrency` | int | Max parallel workers (default: 8) |
| `timeout` | int | Per-item timeout in seconds (default: 30) |
| `dry_run` | bool | Preview without executing |

## Template Variables

Use these in `command`:

| Variable | Value |
|----------|-------|
| `{path}` | Full path to the item |
| `{name}` | Item name |
| `{dir}` | Parent directory |
| `{basename}` | Filename without path |

## Safety

Dry-run is auto-enabled for destructive commands (`push`, `rm`, `delete`, `clean`, `reset`, `--force`). Filter errors silently exclude items rather than aborting the run.

## Output

Returns a JSON summary with collected count, matched count, and per-item results with status and output.
