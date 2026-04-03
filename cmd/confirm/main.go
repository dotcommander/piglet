package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dotcommander/piglet/extensions/confirm"
)

func main() {
	changes := flag.String("changes", "", "comma-separated changed files")
	noTest := flag.Bool("no-test", false, "skip tests")
	noLint := flag.Bool("no-lint", false, "skip lint")
	jsonOut := flag.Bool("json", false, "JSON output")
	flag.Parse()

	opts := confirm.Options{
		NoTest: *noTest,
		NoLint: *noLint,
	}

	// Files from --changes flag or positional args
	if *changes != "" {
		opts.Files = strings.Split(*changes, ",")
	} else if flag.NArg() > 0 {
		opts.Files = flag.Args()
	}

	result, err := confirm.Run(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(confirm.FormatVerdict(result))
	}

	if !result.Pass {
		os.Exit(1)
	}
}
