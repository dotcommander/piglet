// fossil — structured git history queries for LLM agents.
// Usage: fossil <command> [flags] [args]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dotcommander/piglet/extensions/fossil"
)

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "fossil: json encode: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`Usage: fossil <command> [flags] [args]

Commands:
  why <file>[:<start>-<end>]    Blame lines with commit messages
  changes [-since 7d] [-path p] Recent changes summary
  owners [-limit 10] [path]     Code ownership by commit frequency
  cochange [-limit 10] <file>   Files that change alongside <file>
  log [-tokens 1024] [path]     Token-budgeted git log

Flags:
  -json    Output as JSON (all commands except log)

Examples:
  fossil why main.go:42-58
  fossil changes -since 30d
  fossil owners -limit 5 pkg/auth/
  fossil cochange go.mod
  fossil log -tokens 2048
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fossil: getwd: %v\n", err)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "why":
		runWhy(cwd, args)
	case "changes":
		runChanges(cwd, args)
	case "owners":
		runOwners(cwd, args)
	case "cochange":
		runCoChange(cwd, args)
	case "log":
		runLog(cwd, args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "fossil: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}
}

func runWhy(cwd string, args []string) {
	fs := flag.NewFlagSet("why", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "fossil: why requires <file>[:<start>-<end>]\n")
		os.Exit(1)
	}

	arg := fs.Arg(0)
	file, startLine, endLine := parseFileRange(arg)

	results, err := fossil.Why(cwd, file, startLine, endLine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		printJSON(results)
		return
	}

	for _, r := range results {
		fmt.Printf("%s %s %s (lines %s)\n", r.SHA, r.Date, r.Author, r.Lines)
		fmt.Printf("  %s\n", r.Summary)
	}
}

func parseFileRange(arg string) (file string, startLine, endLine int) {
	idx := strings.LastIndex(arg, ":")
	if idx < 0 {
		return arg, 0, 0
	}

	file = arg[:idx]
	rangeStr := arg[idx+1:]

	if startStr, endStr, ok := strings.Cut(rangeStr, "-"); ok {
		start, err1 := strconv.Atoi(startStr)
		end, err2 := strconv.Atoi(endStr)
		if err1 == nil && err2 == nil {
			return file, start, end
		}
	} else {
		n, err := strconv.Atoi(rangeStr)
		if err == nil {
			return file, n, n
		}
	}

	// Could not parse the range part — treat the whole arg as a file path.
	return arg, 0, 0
}

func runChanges(cwd string, args []string) {
	fs := flag.NewFlagSet("changes", flag.ContinueOnError)
	since := fs.String("since", "7d", "Time window (e.g. 7d, 30d)")
	path := fs.String("path", "", "Limit to path")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	results, err := fossil.Changes(cwd, *since, *path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		printJSON(results)
		return
	}

	for _, r := range results {
		fmt.Printf("%s %s: %s\n", r.SHA, r.Author, r.Subject)
		for _, f := range r.Files {
			fmt.Printf("  +%d -%d %s\n", f.Added, f.Deleted, f.File)
		}
	}
}

func runOwners(cwd string, args []string) {
	fs := flag.NewFlagSet("owners", flag.ContinueOnError)
	limit := fs.Int("limit", 10, "Max owners to show")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	path := ""
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	results, err := fossil.Owners(cwd, path, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		printJSON(results)
		return
	}

	for _, r := range results {
		fmt.Printf("%d (%.1f%%)  %s\n", r.Commits, r.Percent, r.Author)
	}
}

func runCoChange(cwd string, args []string) {
	fs := flag.NewFlagSet("cochange", flag.ContinueOnError)
	limit := fs.Int("limit", 10, "Max files to show")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "fossil: cochange requires <file>\n")
		os.Exit(1)
	}
	file := fs.Arg(0)

	results, err := fossil.CoChange(cwd, file, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		printJSON(results)
		return
	}

	for _, r := range results {
		fmt.Printf("%d  %s\n", r.Count, r.File)
	}
}

func runLog(cwd string, args []string) {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	tokens := fs.Int("tokens", 1024, "Token budget")
	path := fs.String("path", "", "Limit to path")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	out, err := fossil.Log(cwd, *tokens, *path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fossil: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(out)
}
