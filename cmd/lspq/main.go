// lspq is a standalone CLI for querying language servers via the lsp package.
//
// Usage: lspq [--json] <command> [flags] <file> [line] [symbol-or-column]
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
	"syscall"

	"github.com/dotcommander/piglet/extensions/lsp"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	// Support both `lspq --json refs ...` and `lspq refs --json ...`.
	// Parse a root-level flag set first, then hand off the remainder.
	root := flag.NewFlagSet("lspq", flag.ContinueOnError)
	jsonFlag := root.Bool("json", false, "Output JSON instead of human-readable text")
	root.SetOutput(os.Stderr)
	// Ignore errors — we want unrecognised flags to fall through to the
	// sub-command flag set so that `-to` and `-col` are still accepted.
	_ = root.Parse(os.Args[1:])
	remaining := root.Args()

	if len(remaining) < 1 {
		usage()
		os.Exit(1)
	}

	cmd := remaining[0]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	to := fs.String("to", "", "New name for rename command")
	col := fs.Int("col", 0, "Column (1-based); auto-detected if symbol name given")
	// Allow --json after the subcommand too.
	jsonSub := fs.Bool("json", false, "Output JSON instead of human-readable text")

	if err := fs.Parse(remaining[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	args := fs.Args()

	jsonMode := *jsonFlag || *jsonSub

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cwd, err := os.Getwd()
	if err != nil {
		fatalf("getwd: %v", err)
	}

	p := runParams{
		Cmd:      cmd,
		Args:     args,
		To:       *to,
		ColFlag:  *col,
		Cwd:      cwd,
		JSONMode: jsonMode,
	}
	if err := run(ctx, p); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// runParams bundles invocation inputs parsed from flags + argv.
type runParams struct {
	Cmd      string   // sub-command: def, refs, hover, rename, symbols
	Args     []string // positional args after the sub-command
	To       string   // -to flag (rename target)
	ColFlag  int      // -col flag (1-based column, 0 = unset)
	Cwd      string   // working directory for path resolution
	JSONMode bool     // --json flag
}

func run(ctx context.Context, p runParams) error {
	mgr := lsp.NewManager(p.Cwd)
	defer mgr.Shutdown(context.Background())

	switch p.Cmd {
	case "symbols":
		return cmdSymbols(ctx, mgr, p)
	case "def", "refs", "hover", "rename":
		return cmdPosition(ctx, mgr, p)
	default:
		usage()
		return fmt.Errorf("unknown command: %q", p.Cmd)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `lspq - LSP query tool

Usage:
  lspq [--json] <command> [flags] <file> [line] [symbol]

Commands:
  def      Go to definition
  refs     Find all references
  hover    Get type info and docs
  rename   Rename symbol (requires -to flag)
  symbols  List all symbols in a file

Flags:
  --json       Output JSON instead of human-readable text
  -to string   New name for rename command
  -col int     Column (1-based); auto-detected if symbol name given

JSON output shapes:
  def:     {"definition": {"file": "...", "line": N, "column": N}}
  refs:    {"references": [{"file": "...", "line": N, "column": N}, ...]}
  hover:   {"hover": "..."}
  symbols: {"symbols": [{"name": "...", "kind": "...", "file": "...", "line": N, "column": N}, ...]}

Examples:
  lspq def main.go 42 HandleRequest
  lspq --json refs server.go 10 -col 5
  lspq hover auth.go 55 Validate
  lspq rename auth.go 55 Validate -to ValidateToken
  lspq symbols main.go
  lspq --json symbols main.go
`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "lspq: "+format+"\n", args...)
	os.Exit(1)
}
