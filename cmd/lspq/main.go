// lspq is a standalone CLI for querying language servers via the lsp package.
//
// Usage: lspq <command> [flags] <file> [line] [symbol-or-column]
//
//	def      Go to definition
//	refs     Find all references
//	hover    Get type info and docs
//	rename   Rename symbol (requires -to flag)
//	symbols  List all symbols in a file
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/dotcommander/piglet/extensions/lsp"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	to := fs.String("to", "", "New name for rename command")
	col := fs.Int("col", 0, "Column (1-based); auto-detected if symbol name given")

	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	args := fs.Args()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cwd, err := os.Getwd()
	if err != nil {
		fatalf("getwd: %v", err)
	}

	if err := run(ctx, cmd, args, *to, *col, cwd); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, args []string, to string, colFlag int, cwd string) error {
	mgr := lsp.NewManager(cwd)
	defer mgr.Shutdown(context.Background())

	switch cmd {
	case "symbols":
		return cmdSymbols(ctx, mgr, args, cwd)
	case "def", "refs", "hover", "rename":
		return cmdPosition(ctx, mgr, cmd, args, to, colFlag, cwd)
	default:
		usage()
		return fmt.Errorf("unknown command: %q", cmd)
	}
}

func cmdSymbols(ctx context.Context, mgr *lsp.Manager, args []string, cwd string) error {
	if len(args) < 1 {
		return fmt.Errorf("symbols requires <file>")
	}
	file := resolveFile(args[0], cwd)

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
	fmt.Println(lsp.FormatSymbols(syms, cwd))
	return nil
}

func cmdPosition(ctx context.Context, mgr *lsp.Manager, cmd string, args []string, to string, colFlag int, cwd string) error {
	if len(args) < 2 {
		return fmt.Errorf("%s requires <file> <line> [symbol]", cmd)
	}

	file := resolveFile(args[0], cwd)

	lineNum, err := strconv.Atoi(args[1])
	if err != nil || lineNum < 1 {
		return fmt.Errorf("line must be a positive integer, got %q", args[1])
	}
	line := lineNum - 1 // convert to 0-based

	col, err := resolveCol(file, line, args[2:], colFlag)
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

	switch cmd {
	case "def":
		locs, err := client.Definition(ctx, file, line, col)
		if err != nil {
			return fmt.Errorf("definition: %w", err)
		}
		fmt.Println(lsp.FormatLocations(locs, cwd, 2))

	case "refs":
		locs, err := client.References(ctx, file, line, col)
		if err != nil {
			return fmt.Errorf("references: %w", err)
		}
		fmt.Println(lsp.FormatLocations(locs, cwd, 1))

	case "hover":
		hover, err := client.Hover(ctx, file, line, col)
		if err != nil {
			return fmt.Errorf("hover: %w", err)
		}
		fmt.Println(lsp.FormatHover(hover))

	case "rename":
		if to == "" {
			return fmt.Errorf("rename requires -to <new-name>")
		}
		edit, err := client.Rename(ctx, file, line, col, to)
		if err != nil {
			return fmt.Errorf("rename: %w", err)
		}
		fmt.Println(lsp.FormatWorkspaceEdit(edit, cwd))
	}

	return nil
}

// resolveCol determines the 0-based column from remaining positional args or the -col flag.
func resolveCol(file string, line int, rest []string, colFlag int) (int, error) {
	if len(rest) > 0 {
		sym := rest[0]
		if _, err := strconv.Atoi(sym); err != nil {
			// Non-numeric: treat as symbol name, auto-detect column.
			return lsp.FindSymbolColumn(file, line, sym)
		}
	}
	if colFlag > 0 {
		return colFlag - 1, nil // convert 1-based to 0-based
	}
	return 0, nil
}

func resolveFile(file, cwd string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(cwd, file)
}

func usage() {
	fmt.Fprint(os.Stderr, `lspq - LSP query tool

Usage:
  lspq <command> [flags] <file> [line] [symbol]

Commands:
  def      Go to definition
  refs     Find all references
  hover    Get type info and docs
  rename   Rename symbol (requires -to flag)
  symbols  List all symbols in a file

Flags:
  -to string   New name for rename command
  -col int     Column (1-based); auto-detected if symbol name given

Examples:
  lspq def main.go 42 HandleRequest
  lspq refs server.go 10 -col 5
  lspq hover auth.go 55 Validate
  lspq rename auth.go 55 Validate -to ValidateToken
  lspq symbols main.go
`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "lspq: "+format+"\n", args...)
	os.Exit(1)
}
