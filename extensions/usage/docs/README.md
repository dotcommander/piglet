# Usage Extension

Tracks and displays token usage statistics for the current session.

## Commands

| Command | Description |
|---------|-------------|
| `/stats` | Show session token usage statistics |
| `/usage` | Alias for `/stats` |

## Tool

| Tool | Description |
|------|-------------|
| `session_stats` | Get current session token usage statistics (returns formatted summary) |

## Event Handler

Listens for `EventTurnUsage` events from the host and accumulates statistics.

## Output Format

```
Session Token Usage
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CUMULATIVE TOTALS
Input                                    2,500
Output                                     800
Cache Read                             100,000
Cache Creation                          5,000

CURRENT PROMPT BREAKDOWN
System Prompt                              800
Repo Map                                 30,000
Tool Definitions                           500
Conversation History                     1,200

EXTENSION PROMPT SECTIONS
memory                                     200
rtk                                        150

Total Turns                                  3
```

## Host Requirements

The host (piglet core) must emit `EventTurnUsage` events with the following payload:

```json
{
  "turn": 3,
  "usage": {
    "input": 1500,
    "output": 600,
    "cache_read": 50000,
    "cache_write": 0,
    "cache_creation": 2000
  },
  "breakdown": {
    "system_prompt": 800,
    "repo_map": 30000,
    "tools": 500,
    "history": 2000,
    "extensions": [
      {"name": "memory", "tokens": 200}
    ]
  }
}
```

## Build

```bash
make extensions-usage
```
