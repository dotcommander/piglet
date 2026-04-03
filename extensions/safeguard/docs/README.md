# Safeguard

Security interceptor that blocks dangerous commands before execution. Acts as a gatekeeper for destructive operations like `rm -rf`, database drops, force pushes, and system-level commands.

## Capabilities

| Capability | Name | Priority | Description |
|------------|------|----------|-------------|
| interceptor | `safeguard` | 2000 | Blocks dangerous bash commands and (in strict mode) file writes outside CWD |

## Profiles

| Profile | Behavior |
|---------|----------|
| **strict** | Blocks dangerous patterns + enforces workspace scoping (write/edit outside CWD rejected) |
| **balanced** | Blocks dangerous patterns only (default) |
| **off** | Disabled — logs only |

## Configuration

File: `~/.config/piglet/safeguard.yaml`

```yaml
profile: balanced
patterns:
  - \brm\s+-(r|f|rf|fr)\b
  - \bsudo\s+rm\b
  # ... more patterns
```

Auto-created with defaults if missing.

## Default Blocked Patterns (18 rules)

- **File destruction**: `rm -rf`, `rm -r /`, `sudo rm`
- **Disk ops**: `mkfs`, `dd if=`
- **Database**: `DROP TABLE/DATABASE`, `TRUNCATE`, `DELETE FROM`
- **Git force ops**: `push --force`, `reset --hard`, `clean -dfx`, `branch -D`
- **Permissions**: `chmod -R 777`, `chown -R`
- **System**: `shutdown`, `reboot`, `systemctl stop/disable/mask`, `kill -9 -1`
- **Device writes**: `> /dev/sda*`
- **Fork bombs**: `:(){ :|:& };:`

## Audit Log

Blocked actions are logged to `~/.config/piglet/safeguard-audit.jsonl` in JSONL format with timestamp, tool, decision, reason, and detail.

## Workspace Scoping (strict mode)

In strict mode, `write`, `edit`, and `multi_edit` tool calls are validated to ensure the target path is inside the current working directory. Correctly handles directory boundary attacks (e.g., `/workspace-evil/` won't match `/workspace/`).
