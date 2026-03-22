// LSP extension binary. Provides code intelligence via language servers.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/lsp"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

var mgr *lsp.Manager

func main() {
	e := sdk.New("lsp", "0.1.0")

	e.OnInit(func(x *sdk.Extension) {
		mgr = lsp.NewManager(x.CWD())

		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title: "Code Intelligence (LSP)",
			Content: strings.Join([]string{
				"Tools: lsp_definition, lsp_references, lsp_hover, lsp_rename, lsp_symbols",
				"",
				"These tools provide precise code intelligence via language servers.",
				"Prefer lsp_definition over grep for finding where functions/types are defined.",
				"Prefer lsp_references over grep for finding all usages of a symbol.",
				"Use lsp_hover to get type signatures and documentation.",
				"Use lsp_rename for safe cross-file symbol renaming.",
				"",
				"All tools accept file + line (1-based). Use symbol param instead of column",
				"when you know the symbol name — the tool finds its column automatically.",
				"",
				"Supported languages: Go (gopls), TypeScript/JS, Python, Rust, C/C++, Java, Lua, Zig.",
				"Language servers must be installed and in PATH.",
			}, "\n"),
			Order: 40,
		})
	})

	positionParams := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file":   map[string]any{"type": "string", "description": "File path (absolute or relative to project root)"},
			"line":   map[string]any{"type": "integer", "description": "Line number (1-based)"},
			"column": map[string]any{"type": "integer", "description": "Column number (1-based). Optional if symbol is provided."},
			"symbol": map[string]any{"type": "string", "description": "Symbol name to find on the line. Used to auto-detect column."},
		},
		"required": []string{"file", "line"},
	}

	e.RegisterTool(sdk.ToolDef{
		Name:        "lsp_definition",
		Description: "Go to the definition of a symbol. Returns the file and line where the symbol is defined.",
		Parameters:  positionParams,
		PromptHint:  "Find where a function, type, or variable is defined",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			client, file, line, col, err := prepareClient(ctx, args)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			locs, err := client.Definition(ctx, file, line, col)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("definition: %v", err)), nil
			}
			return sdk.TextResult(lsp.FormatLocations(locs, mgr.CWD(), 2)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "lsp_references",
		Description: "Find all references to a symbol across the codebase. Returns file paths and line numbers.",
		Parameters:  positionParams,
		PromptHint:  "Find all usages of a function, type, or variable",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			client, file, line, col, err := prepareClient(ctx, args)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			locs, err := client.References(ctx, file, line, col)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("references: %v", err)), nil
			}
			return sdk.TextResult(lsp.FormatLocations(locs, mgr.CWD(), 1)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "lsp_hover",
		Description: "Get type information and documentation for a symbol.",
		Parameters:  positionParams,
		PromptHint:  "Get type signature and docs for a symbol",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			client, file, line, col, err := prepareClient(ctx, args)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			hover, err := client.Hover(ctx, file, line, col)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("hover: %v", err)), nil
			}
			return sdk.TextResult(lsp.FormatHover(hover)), nil
		},
	})

	renameParams := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file":     map[string]any{"type": "string", "description": "File path (absolute or relative to project root)"},
			"line":     map[string]any{"type": "integer", "description": "Line number (1-based)"},
			"column":   map[string]any{"type": "integer", "description": "Column number (1-based). Optional if symbol is provided."},
			"symbol":   map[string]any{"type": "string", "description": "Symbol name to find on the line. Used to auto-detect column."},
			"new_name": map[string]any{"type": "string", "description": "New name for the symbol"},
		},
		"required": []string{"file", "line", "new_name"},
	}

	e.RegisterTool(sdk.ToolDef{
		Name:        "lsp_rename",
		Description: "Rename a symbol across the entire codebase. Returns a preview of all changes (does not apply them).",
		Parameters:  renameParams,
		PromptHint:  "Rename a symbol across the codebase (preview only)",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			client, file, line, col, err := prepareClient(ctx, args)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			newName, _ := args["new_name"].(string)
			if newName == "" {
				return sdk.ErrorResult("new_name is required"), nil
			}
			edit, err := client.Rename(ctx, file, line, col, newName)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("rename: %v", err)), nil
			}
			return sdk.TextResult(lsp.FormatWorkspaceEdit(edit, mgr.CWD())), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "lsp_symbols",
		Description: "List all symbols (functions, types, variables) defined in a file.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file": map[string]any{"type": "string", "description": "File path (absolute or relative to project root)"},
			},
			"required": []string{"file"},
		},
		PromptHint: "List functions, types, and variables in a file",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			file := resolveFile(args)
			if file == "" {
				return sdk.ErrorResult("file is required"), nil
			}
			client, lang, err := mgr.ForFile(ctx, file)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			if err := mgr.EnsureFileOpen(ctx, client, file, lang); err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			symbols, err := client.DocumentSymbols(ctx, file)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("symbols: %v", err)), nil
			}
			return sdk.TextResult(lsp.FormatSymbols(symbols, mgr.CWD())), nil
		},
	})

	defer func() {
		if mgr != nil {
			mgr.Shutdown(context.Background())
		}
	}()
	e.Run()
}

// prepareClient resolves position args, gets the LSP client, and opens the file.
func prepareClient(ctx context.Context, args map[string]any) (*lsp.Client, string, int, int, error) {
	file, line, col, err := resolvePosition(args)
	if err != nil {
		return nil, "", 0, 0, err
	}
	client, lang, err := mgr.ForFile(ctx, file)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if err := mgr.EnsureFileOpen(ctx, client, file, lang); err != nil {
		return nil, "", 0, 0, err
	}
	return client, file, line, col, nil
}

// resolvePosition extracts file, line (0-based), and column (0-based) from tool args.
func resolvePosition(args map[string]any) (file string, line, col int, err error) {
	file = resolveFile(args)
	if file == "" {
		return "", 0, 0, fmt.Errorf("file is required")
	}

	lineF, _ := args["line"].(float64)
	line = int(lineF) - 1 // convert 1-based to 0-based
	if line < 0 {
		return "", 0, 0, fmt.Errorf("line must be >= 1")
	}

	if colF, ok := args["column"].(float64); ok && colF > 0 {
		col = int(colF) - 1
		return file, line, col, nil
	}

	if symbol, ok := args["symbol"].(string); ok && symbol != "" {
		col, err = lsp.FindSymbolColumn(file, line, symbol)
		if err != nil {
			return "", 0, 0, err
		}
		return file, line, col, nil
	}

	return file, line, 0, nil
}

func resolveFile(args map[string]any) string {
	file, _ := args["file"].(string)
	if file == "" {
		return ""
	}
	if mgr != nil && !filepath.IsAbs(file) {
		file = filepath.Join(mgr.CWD(), file)
	}
	return file
}
