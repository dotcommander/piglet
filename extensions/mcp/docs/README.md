# MCP

Integrates Model Context Protocol (MCP) servers with piglet. Connects to configured servers, discovers their tools, and exposes them as piglet tools.

## Capabilities

| Capability | Name | Description |
|------------|------|-------------|
| tools | `mcp__<server>__<tool>` | Dynamically discovered from connected MCP servers |
| command | `/mcp` | Show status of all connected servers |
| prompt | "MCP Server Instructions" | Initialization instructions from servers |

## Prompt Order

80

## Configuration

File: `~/.config/piglet/mcp.yaml`

```yaml
servers:
  my-server:
    type: stdio
    command: node
    args: ["server.js"]
    env:
      API_KEY: ${API_KEY}    # supports env var expansion

  remote-server:
    type: http
    url: https://example.com/mcp
    headers:
      Authorization: Bearer ${TOKEN}
```

## How It Works

1. On init, reads `mcp.yaml` and connects to all configured servers in parallel
2. Each server connection uses JSON-RPC (protocol version `2025-03-26`)
3. Discovers tools from each server and registers them with piglet as `mcp__<server>__<tool>`
4. Tool calls are proxied through the MCP client; text results are extracted and returned
5. Server instructions are collected and injected into the system prompt

## Transport

| Type | Protocol |
|------|----------|
| `stdio` | Spawns subprocess, communicates via stdin/stdout |
| `http` | HTTP/SSE connection to remote server |

## Failure Behavior

Partial failures are tolerated — if one server fails to connect, others still work. Connection timeout: 30 seconds.
