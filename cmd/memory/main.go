// memory — standalone CLI for per-project key-value memory.
// Usage: memory <command> [args]
package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/dotcommander/piglet/cmd/internal/cliutil"
	"github.com/dotcommander/piglet/extensions/memory"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: memory [-dir <path>] [-json] <command> [args]

Commands:
  get <key>                    get a fact by key
  set <key> <value> [category] set a fact
  list [category]              list all facts, optionally filtered by category
  delete <key>                 delete a fact by key
  clear                        clear all facts
  path                         print the backing file path

Flags:
  -dir string   working directory for store lookup (default: current directory)
  -json         output as JSON

Examples:
  memory set api_url "https://api.example.com" config
  memory get api_url
  memory list
  memory list config
  memory delete api_url
  memory path
`)
}

func main() {
	// Parse -dir and -json flags manually before the subcommand so that
	// flag parsing does not consume subcommand arguments.
	dir := ""
	asJSON := false
	args := os.Args[1:]

	for len(args) > 0 {
		switch args[0] {
		case "-dir", "--dir":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "memory: -dir requires an argument")
				os.Exit(1)
			}
			dir = args[1]
			args = args[2:]
		case "-json", "--json":
			asJSON = true
			args = args[1:]
		default:
			if v, ok := strings.CutPrefix(args[0], "-dir="); ok {
				dir = v
				args = args[1:]
			} else if v, ok := strings.CutPrefix(args[0], "--dir="); ok {
				dir = v
				args = args[1:]
			} else {
				// Not a known flag — stop flag parsing, treat rest as subcommand.
				goto doneFlags
			}
		}
	}
doneFlags:

	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "memory: getwd: %v\n", err)
			os.Exit(1)
		}
		dir = cwd
	}

	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	store, err := memory.NewStore(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memory: open store: %v\n", err)
		os.Exit(1)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "get":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "memory: get requires <key>")
			os.Exit(1)
		}
		fact, ok := store.Get(rest[0])
		if !ok {
			fmt.Fprintf(os.Stderr, "memory: key %q not found\n", rest[0])
			os.Exit(1)
		}
		if asJSON {
			cliutil.PrintJSON(fact)
			return
		}
		fmt.Println(fact.Value)

	case "set":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "memory: set requires <key> <value> [category]")
			os.Exit(1)
		}
		category := ""
		if len(rest) >= 3 {
			category = rest[2]
		}
		if err := store.Set(rest[0], rest[1], category); err != nil {
			fmt.Fprintf(os.Stderr, "memory: set: %v\n", err)
			os.Exit(1)
		}

	case "list":
		category := ""
		if len(rest) >= 1 {
			category = rest[0]
		}
		facts := store.List(category)
		if asJSON {
			cliutil.PrintJSON(facts)
			return
		}
		if len(facts) == 0 {
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tVALUE\tCATEGORY\tUPDATED")
		for _, f := range facts {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				f.Key,
				f.Value,
				f.Category,
				f.UpdatedAt.Format("2006-01-02 15:04:05"),
			)
		}
		w.Flush()

	case "delete":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "memory: delete requires <key>")
			os.Exit(1)
		}
		if err := store.Delete(rest[0]); err != nil {
			fmt.Fprintf(os.Stderr, "memory: delete: %v\n", err)
			os.Exit(1)
		}

	case "clear":
		if err := store.Clear(); err != nil {
			fmt.Fprintf(os.Stderr, "memory: clear: %v\n", err)
			os.Exit(1)
		}

	case "path":
		fmt.Println(store.Path())

	default:
		fmt.Fprintf(os.Stderr, "memory: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}
}
