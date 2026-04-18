# Extensions

- [Overview](#overview)
- [How Extensions Work](#how-extensions-work)
- [Extension Packs](#extension-packs)
- [Installing Extensions](#installing-extensions)
- [Listing Extensions](#listing-extensions)
- [Disabling Extensions](#disabling-extensions)
- [Extension Loading](#extension-loading)
- [Project-Local Extensions](#project-local-extensions)
- [Prompt Templates](#prompt-templates)
- [Troubleshooting](#troubleshooting)
- [Building Extensions](#building-extensions)

## Overview

Piglet is extension-first. The core binary is a minimal agent loop — every capability beyond "stream LLM, execute tools, repeat" lives in an extension.

Extensions can add:

- **Tools** — functions the LLM can call (file search, web fetch, memory, etc.)
- **Commands** — slash commands the user types (`/export`, `/undo`, `/extensions`)
- **Shortcuts** — keyboard shortcuts (`Ctrl+G` for git status, etc.)
- **Prompt sections** — text injected into the system prompt (memory, skills, git context)
- **Interceptors** — before/after hooks on tool calls (safeguard, RTK token optimization)
- **Event handlers** — observers for agent lifecycle (auto-title, usage tracking)
- **Message hooks** — pre-processing of user messages before the LLM sees them
- **Input transformers** — rewrite or consume user input before the agent
- **Compactors** — conversation compaction strategy (LLM-based summarization)
- **Stream providers** — custom LLM streaming backends

Without extensions, piglet starts with 4 built-in tools (`read`, `write`, `edit`, `bash`) and 7 built-in commands. With extensions installed, the full feature set is available (`grep`, `find`, `ls` are provided by pack-code; session/model commands are provided by pack-context/sessioncmd).

## How Extensions Work

There are two kinds of extensions:

### Built-In Extensions

Compiled directly into the piglet binary. They load instantly and provide the core tools and commands. Found in `tool/`, `command/`, and `prompt/` in the source tree.

### External Extensions

Run as standalone binaries alongside piglet, communicating via JSON-RPC v2. They can be written in Go, TypeScript, Python, or any language. External extensions are organized into **packs** — single binaries that bundle multiple related extensions for fast startup.

Both kinds use the same `ext.App` registration API. There is no privileged access — a built-in tool and an external tool are indistinguishable to the agent.

## Extension Packs

Official extensions are consolidated into packs. Each pack is a single binary that registers capabilities from multiple logical extensions:

| Pack | Contains | What It Adds |
|------|----------|-------------|
| **pack-context** | memory, skill, gitcontext, behavior, prompts, session-tools, inbox | Memory management, skills, git context in prompts, behavioral guidelines |
| **pack-code** | lsp, repomap, sift, plan, suggest | LSP integration, repository maps, code search, planning tools |
| **pack-agent** | safeguard, rtk, autotitle, clipboard, subagent, provider, loop | Dangerous command blocking, token optimization, auto-titling, sub-agents |
| **pack-core** | admin, export, extensions-list, undo, scaffold, background | Admin commands, export, undo, extension scaffolding |
| **pack-workflow** | pipeline, bulk, webfetch, cache, usage, modelsdev | Pipeline runner, bulk operations, web fetching, usage tracking |
| **pack-cron** | cron | Scheduled task execution |
| **mcp** | mcp | Bridge to MCP (Model Context Protocol) servers |

### Which Packs Are Required?

| Pack | Importance | Impact If Disabled |
|------|------------|-------------------|
| **pack-context** | Essential | No memory, no skills, no git context, no behavioral guidelines |
| **pack-code** | Essential | No LSP, no repo maps, no code-aware search |
| **pack-agent** | Important | No safeguard (dangerous command blocking), no auto-titles, no sub-agents |
| **pack-core** | Convenient | No `/export`, `/undo`, `/extensions list`, extension scaffolding |
| **pack-workflow** | Optional | No `/pipe`, `/bulk`, web fetch |
| **pack-cron** | Optional | No scheduled tasks |
| **mcp** | Optional | No MCP server bridging |

## Installing Extensions

Extensions install automatically on the first interactive launch. If you need to install or update manually:

### From Inside Piglet

```
/update                    # Upgrade piglet binary + rebuild all extensions
```

### From the Command Line

```bash
piglet update              # Same as /update
piglet update --local      # Build from local checkout (development)
```

### What Happens During Install

1. Piglet shallow-clones the `piglet-extensions` repository
2. Builds each pack binary with `go build`
3. Installs binaries and manifest files to `~/.config/piglet/extensions/`
4. Caches the remote commit hash for incremental updates

On subsequent runs, piglet checks the remote HEAD against the cached hash. If unchanged, it skips the build entirely.

## Listing Extensions

Use the `/extensions` command (provided by pack-core) to see all loaded extensions:

```
/extensions
```

This shows each extension's name, version, kind (builtin/external), and what it registered (tools, commands, interceptors, etc.).

## Disabling Extensions

To disable an extension, add its name to the `disabled_extensions` list in `~/.config/piglet/config.yaml`:

```yaml
disabled_extensions:
  - pack-workflow
  - pack-cron
```

Then restart piglet. Disabled extensions are skipped during loading.

### Finding Extension Names

Run `/extensions` to see all loaded extension names. The name to use in `disabled_extensions` is the `Name` field shown in the listing (e.g., `pack-core`, `pack-agent`, `mcp`).

### Granularity

Extensions are disabled at the **pack level**. You cannot disable individual components within a pack — it's all or nothing. For example, disabling `pack-agent` disables safeguard, auto-title, RTK, sub-agents, and everything else in that pack.

If you need finer control, you would need to build a custom pack that excludes specific components. See [Building Extensions](building-extensions.md) for details.

### Verifying

After restarting, run `/extensions` to confirm the disabled extensions are no longer loaded. Piglet also logs `"extension disabled by config"` for each skipped extension (visible in debug mode).

## Extension Loading

### Load Order

1. **Built-in tools** — `read`, `write`, `edit`, `bash`
2. **Built-in commands** — `/help`, `/clear`, `/compact`, `/step`, `/update`, `/upgrade`, `/quit`
3. **Built-in prompt sections** — self-knowledge
4. **External extensions** — from `~/.config/piglet/extensions/`, alphabetical by directory name

Later registrations with the same name overwrite earlier ones. This means external extensions can replace built-in tools and commands.

### Interactive vs One-Shot

In **interactive mode** (TUI), external extensions load asynchronously in the background. The TUI appears immediately — you can start typing while extensions load (~1 second).

In **one-shot mode** (`piglet "prompt"`), extensions load synchronously before the first message to ensure all tools are available.

### Crash Recovery

Each external extension runs under a supervisor that:

- Detects unexpected process exit
- Removes stale registrations from the app
- Restarts with exponential backoff (up to 5 attempts)
- Resets the failure counter after 30 seconds of uptime
- Sends a notification if restart fails permanently

### Hot Reload

The supervisor supports intentional restart via `Reload()`. When an extension binary changes on disk, the supervisor gracefully stops the old process and starts the new one — no piglet restart needed.

## Project-Local Extensions

Projects can ship their own extensions in `.piglet/extensions/`:

```
my-project/
  .piglet/
    extensions/
      custom-tool/
        manifest.yaml
        main.go
```

Project-local extensions **override global extensions** with the same name. This is disabled by default for security — enable it in `config.yaml`:

```yaml
allowProjectExtensions: true
```

Only enable this for projects you trust. Project-local extensions execute arbitrary code.

## Prompt Templates

Prompt templates are a lightweight alternative to full extensions. Create a markdown file and it becomes a slash command:

### Creating a Template

Create `~/.config/piglet/prompts/review.md`:

```markdown
---
description: Review code for issues
---
Review the following code for bugs, security issues, and style problems:

$@
```

Now `/review main.go` expands the template with your arguments and sends it to the agent.

### Argument Substitution

| Placeholder | Meaning |
|-------------|---------|
| `$1`, `$2`, ... `$9` | Positional arguments |
| `$@` | All arguments joined by space |
| `${@:N}` | Arguments from position N onward |
| `${@:N:L}` | L arguments starting from position N |

### Template Locations

| Location | Scope |
|----------|-------|
| `~/.config/piglet/prompts/` | Global — available in all projects |
| `.piglet/prompts/` | Project-local — override global templates |

Project-local templates take precedence when names collide.

## Troubleshooting

### Extensions didn't install

Check that `go` and `git` are on your PATH:

```bash
which go git
```

If either is missing, piglet skips extension installation gracefully. Install them, then run `/update` from inside piglet.

### Extension won't load

1. Check the manifest: `cat ~/.config/piglet/extensions/<name>/manifest.yaml`
2. Verify the runtime is available: `which bun` / `which node` / `which python3`
3. Check for errors in debug mode: `piglet --debug`, then look at `~/.config/piglet/debug.log`
4. Try running the extension manually: `cd ~/.config/piglet/extensions/<name> && ./<binary>`

### Extension keeps crashing

The supervisor retries up to 5 times with exponential backoff. If an extension fails permanently, you'll see a notification in the TUI. Common causes:

- Missing dependencies (run `bun install` or `npm install` in the extension directory)
- Incompatible SDK version (run `/update` to rebuild)
- Runtime errors (check debug log)

### Extension disabled but still loading

Make sure the name in `disabled_extensions` matches exactly. Names are case-sensitive and must match the `name` field in `manifest.yaml`, not the directory name.

### Wrong tools showing up

If you have an old extension binary that wasn't cleaned up, it may still register. Run `/update` to rebuild all extensions, which cleans up stale binaries.

## Building Extensions

For a complete guide to building your own extensions, see [Building Extensions](building-extensions.md).

For guidance on when to use native extensions vs MCP servers, see [Extensions vs MCP](extensions-vs-mcp.md).
