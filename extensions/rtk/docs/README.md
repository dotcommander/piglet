# RTK

Token optimization proxy that rewrites bash commands through [RTK](https://github.com/dotcommander/rtk) (Rust Token Killer) to reduce LLM token usage by 60-90%.

## Capabilities

| Capability | Name | Priority | Description |
|------------|------|----------|-------------|
| interceptor | `rtk` | 100 | Rewrites bash commands before execution |
| prompt | "RTK Token Optimization" | 90 | Informs the LLM that commands are auto-optimized |

## How It Works

1. On startup, checks if `rtk` binary is in PATH via `exec.LookPath()`
2. If found, registers a before-interceptor on `bash` tool calls
3. Before each bash command: calls `rtk rewrite <command>` and replaces the command if the output differs
4. If RTK is not found or rewrite fails, passes through the original command unchanged

## Configuration

No config file required. Behavior is controlled by the `rtk` key in piglet settings:

| Value | Behavior |
|-------|----------|
| omitted | Auto-detect — use if in PATH, silent no-op otherwise |
| `true` | Require RTK in PATH |
| `false` | Disable entirely |

## Failure Behavior

Silently degrades — errors in rewrite never block command execution.
