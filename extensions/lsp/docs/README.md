# LSP

Provides precise code navigation and refactoring via language servers. Acts as a bridge between the LLM and LSP-compliant language servers running locally.

## Capabilities

| Capability | Name | Description |
|------------|------|-------------|
| tool | `lsp_definition` | Go to symbol definition |
| tool | `lsp_references` | Find all usages of a symbol |
| tool | `lsp_hover` | Get type signature and docs |
| tool | `lsp_rename` | Preview cross-file symbol rename |
| tool | `lsp_symbols` | List all symbols in a file |
| prompt | "Code Intelligence (LSP)" | Guidance on when to prefer LSP over grep |

## Prompt Order

40

## Supported Languages

Go, TypeScript/JavaScript, Python, Rust, C/C++, Java, Lua, Zig.

## How It Works

1. On init, creates a `Manager` at the project working directory
2. Language servers are spawned on-demand (one per language)
3. Tool calls provide file + line + symbol name — the manager routes to the correct server
4. Results are formatted with context lines, hover docs, rename previews, or symbol trees

## Key Design

- Stateful client lifecycle: initialize → didOpen → requests → shutdown
- `lsp_rename` only previews changes — never applies them
- Symbol parameter auto-detects column position via `FindSymbolColumn()`
