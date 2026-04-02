# Extensions vs MCP

Piglet supports two ways to add tools: native extensions and MCP servers. This guide helps you choose.

## Decision Tree

1. **Do you need TUI integration?** (shortcuts, status bar, interceptors, prompt sections, event handlers, message hooks, compaction, provider streaming) → **Extension**
2. **Do you have an existing MCP server?** Or need the tool to work in Claude Code, Cursor, etc. too? → **MCP**
3. **Need both?** → Extension that bridges MCP internally (like piglet's `mcp` extension does)

## Comparison

| Capability | Extension | MCP |
|-----------|-----------|-----|
| Tools (LLM-callable) | Yes | Yes |
| Slash commands | Yes | No |
| Prompt sections | Yes | No |
| Interceptors (before/after) | Yes | No |
| Event handlers | Yes | No |
| Keyboard shortcuts | Yes | No |
| Message hooks | Yes | No |
| Compactor | Yes | No |
| Provider streaming | Yes | No |
| Cross-app compatible | No (piglet only) | Yes (any MCP host) |
| Protocol | JSON-RPC over FD 3/4 | JSON-RPC over stdio/HTTP |
| Language | Any (Go SDK provided) | Any |
| Auto-restart on crash | Yes (supervised) | No |

## When to Use MCP

- You already have an MCP server (database tools, API wrappers, file systems)
- You want the same tool in multiple AI assistants (Claude Code, Cursor, Windsurf)
- The tool only needs request/response — no lifecycle hooks

Configure MCP servers in `~/.config/piglet/mcp.yaml`. They appear as `mcp__<server>__<tool>` in the tool list.

## When to Use Extensions

- You need any TUI integration (status bar updates, keyboard shortcuts, interceptors)
- You need agent lifecycle hooks (react to turn end, session start, compaction)
- You need to modify or block tool calls (interceptors)
- You need to inject context before the LLM sees messages (message hooks)
- You need streaming (provider protocol)
- You want deep piglet integration with auto-restart on crash

See [Building Extensions](building-extensions.md) for the full extension development guide.

## Examples

| Goal | Use |
|------|-----|
| Add a database query tool | MCP (portable, request/response only) |
| Block dangerous shell commands | Extension (interceptor) |
| Auto-title sessions | Extension (event handler) |
| Inject git context into prompts | Extension (prompt section) |
| Paste images with Ctrl+V | Extension (shortcut + tool) |
| Bridge an existing MCP server | Extension (`mcp` extension already does this) |
| Add an LLM provider | Extension (provider streaming) |
