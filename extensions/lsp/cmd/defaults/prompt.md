Tools: lsp_definition, lsp_references, lsp_hover, lsp_rename, lsp_symbols

These tools provide precise code intelligence via language servers.
Prefer lsp_definition over grep for finding where functions/types are defined.
Prefer lsp_references over grep for finding all usages of a symbol.
Use lsp_hover to get type signatures and documentation.
Use lsp_rename for safe cross-file symbol renaming.

All tools accept file + line (1-based). Use symbol param instead of column
when you know the symbol name — the tool finds its column automatically.

Supported languages: Go (gopls), TypeScript/JS, Python, Rust, C/C++, Java, Lua, Zig.
Language servers must be installed and in PATH.