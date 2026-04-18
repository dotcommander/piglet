package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dotcommander/piglet/extensions/lsp"
)

func cmdSymbols(ctx context.Context, mgr *lsp.Manager, p runParams) error {
	if len(p.Args) < 1 {
		return fmt.Errorf("symbols requires <file>")
	}
	file := resolveFile(p.Args[0], p.Cwd)

	client, lang, err := mgr.ForFile(ctx, file)
	if err != nil {
		return err
	}
	if err := mgr.EnsureFileOpen(ctx, client, file, lang); err != nil {
		return err
	}
	syms, err := client.DocumentSymbols(ctx, file)
	if err != nil {
		return fmt.Errorf("symbols: %w", err)
	}

	if p.JSONMode {
		return writeJSON(buildSymbolsJSON(syms, file))
	}
	fmt.Println(lsp.FormatSymbols(syms, p.Cwd))
	return nil
}

func cmdPosition(ctx context.Context, mgr *lsp.Manager, p runParams) error {
	if len(p.Args) < 2 {
		return fmt.Errorf("%s requires <file> <line> [symbol]", p.Cmd)
	}

	file := resolveFile(p.Args[0], p.Cwd)

	lineNum, err := strconv.Atoi(p.Args[1])
	if err != nil || lineNum < 1 {
		return fmt.Errorf("line must be a positive integer, got %q", p.Args[1])
	}
	line := lineNum - 1 // convert to 0-based

	col, err := resolveCol(file, line, p.Args[2:], p.ColFlag)
	if err != nil {
		return err
	}

	client, lang, err := mgr.ForFile(ctx, file)
	if err != nil {
		return err
	}
	if err := mgr.EnsureFileOpen(ctx, client, file, lang); err != nil {
		return err
	}

	switch p.Cmd {
	case "def":
		locs, err := client.Definition(ctx, file, line, col)
		if err != nil {
			return fmt.Errorf("definition: %w", err)
		}
		if p.JSONMode {
			return writeJSON(buildDefJSON(locs, p.Cwd))
		}
		fmt.Println(lsp.FormatLocations(locs, p.Cwd, 2))

	case "refs":
		locs, err := client.References(ctx, file, line, col)
		if err != nil {
			return fmt.Errorf("references: %w", err)
		}
		if p.JSONMode {
			return writeJSON(buildRefsJSON(locs, p.Cwd))
		}
		fmt.Println(lsp.FormatLocations(locs, p.Cwd, 1))

	case "hover":
		hover, err := client.Hover(ctx, file, line, col)
		if err != nil {
			return fmt.Errorf("hover: %w", err)
		}
		if p.JSONMode {
			return writeJSON(buildHoverJSON(hover))
		}
		fmt.Println(lsp.FormatHover(hover))

	case "rename":
		if p.To == "" {
			return fmt.Errorf("rename requires -to <new-name>")
		}
		edit, err := client.Rename(ctx, file, line, col, p.To)
		if err != nil {
			return fmt.Errorf("rename: %w", err)
		}
		fmt.Println(lsp.FormatWorkspaceEdit(edit, p.Cwd))
	}

	return nil
}
