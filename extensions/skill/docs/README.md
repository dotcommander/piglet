# Skill

On-demand methodology loading system. Scans markdown files from the skills directory, parses YAML frontmatter, and surfaces them via tools, commands, message hooks, and the system prompt.

## Capabilities

| Capability | Name | Description |
|------------|------|-------------|
| tool | `skill_list` | List available skills with descriptions and triggers |
| tool | `skill_load` | Load full skill content by name |
| command | `/skill` | `/skill list` or `/skill <name>` to load a skill |
| prompt | "Skills" | Lists available skills in the system prompt |
| messageHook | `skill-trigger` | Auto-loads matching skill when trigger keywords appear in user message |

## Prompt Order

25

## Skill File Format

Skills are markdown files in `~/.config/piglet/skills/` with optional YAML frontmatter:

```markdown
---
name: my-skill
description: What this skill does
triggers:
  - keyword1
  - keyword2
---

Skill content goes here...
```

If frontmatter is missing, the filename is used as the skill name.

## Trigger Matching

- Case-insensitive keyword matching against user messages
- When multiple skills match, the longest trigger keyword wins (most specific first)
- Priority: 500

## Failure Behavior

Skips registration entirely if the skills directory is missing or empty. No errors, no warnings.
