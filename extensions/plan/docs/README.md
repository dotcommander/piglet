# Plan

Persistent structured task tracking for multi-step work with checkpoint commits and resumability.

## Capabilities

| Capability | Name | Description |
|------------|------|-------------|
| tool | `plan_create` | Create a new plan with steps |
| tool | `plan_update` | Update step status, notes, add/remove steps |
| tool | `plan_mode` | Switch between propose and execute modes |
| command | `/plan` | Manage plans via slash commands |
| prompt | Active plan | Injects active plan into system prompt |
| interceptor | Plan mode gate | Blocks mutating tools in propose mode |

## Prompt Order

55

## Checkpoint Commits

When working in a git repository, the plan extension automatically creates checkpoint commits when steps reach terminal status (done, skipped, failed). This ensures work is never lost and enables resumability.

**Commit format:** `[plan:<slug>] step <id>: <step text>`

**Enable/disable:**
```
/plan checkpoints    # Toggle checkpoint commits on/off
```

**Default:** Enabled automatically when in a git repo.

## Resumability

Plans are stored in `~/.config/piglet/plans/<cwd-hash>/` as YAML files. On load:
- The **resume point** shows the next incomplete step
- **Last checkpoint** shows the most recent commit SHA
- Progress persists across sessions

```
/plan resume    # Show resume point and last checkpoint
```

## Command Usage

```
/plan                # show active plan with resume point
/plan list           # list all plans with progress
/plan switch <slug>  # activate a plan
/plan archive        # deactivate active plan
/plan clear          # delete active plan
/plan delete <slug>  # delete specific plan
/plan approve        # switch to execute mode
/plan mode           # show current mode
/plan checkpoints    # toggle checkpoint commits
/plan resume         # show resume point
```

## Tool Parameters

### plan_create

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `title` | string | yes | Plan title |
| `steps` | array | yes | Step descriptions in order |
| `checkpoints` | boolean | no | Enable checkpoint commits (default: true in git repos) |

### plan_update

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `step` | int | yes | Step ID to operate on |
| `status` | enum | no | `pending`, `in_progress`, `done`, `skipped`, `failed` |
| `notes` | string | no | Freeform notes on this step |
| `add_after` | string | no | Add a new step after this step ID |
| `remove` | boolean | no | Remove this step |
| `checkpoint` | boolean | no | Force checkpoint commit |

## Modes

| Mode | Behavior |
|------|----------|
| **execute** | Normal operation — all tools allowed, checkpoints on terminal status |
| **propose** | Blocks write, edit, bash — records proposed changes as plan steps |

## Step Statuses

`pending` → `in_progress` → `done` / `skipped` / `failed`

Setting a step to `in_progress` auto-reverts any previous in-progress step to pending. Plans auto-archive when all steps reach a terminal status.

## Example Output

```
## Active Plan: Refactor Auth System

▶ Resume: step 3 — Update password hashing

- [x] 1. Add password validation (abc1234)
- [x] 2. Create migration script (def5678)
- [ ] **3. Update password hashing** ← in progress
- [ ] 4. Add rate limiting
- [ ] 5. Update tests

Progress: 2/5 done | checkpoints enabled
```

## Storage

Plans stored as YAML in `~/.config/piglet/plans/<cwd-hash>/`. One file per plan, keyed by auto-generated slug. Thread-safe with RWMutex; atomic writes via temp file + rename.
