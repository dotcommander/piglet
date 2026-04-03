// bulk — run a shell command across many items in parallel.
// Usage: bulk [flags] <command>
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dotcommander/piglet/extensions/bulk"
)

func main() {
	fs := flag.NewFlagSet("bulk", flag.ExitOnError)
	fs.Usage = usage

	useGit := fs.Bool("git", false, "scan for git repos")
	useDirs := fs.Bool("dirs", false, "scan for directories")
	useFiles := fs.Bool("files", false, "scan for files by glob")
	listArg := fs.String("list", "", "comma-separated explicit paths")

	root := fs.String("root", ".", "root directory")
	pattern := fs.String("pattern", "", "match pattern (dirs: filename to find; files: glob)")
	filterArg := fs.String("filter", "", "shell predicate or git filter (ahead/behind/dirty/clean/diverged)")
	depth := fs.Int("depth", 1, "scan depth")
	concurrency := fs.Int("j", 8, "concurrency")
	timeoutSec := fs.Int("timeout", 30, "per-item timeout in seconds")
	dryRun := fs.Bool("dry-run", false, "collect and filter without executing")
	asJSON := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	args := fs.Args()
	if len(args) == 0 && !*dryRun {
		fmt.Fprintln(os.Stderr, "bulk: command required")
		fs.Usage()
		os.Exit(2)
	}

	command := strings.Join(args, " ")

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve root: %v", err)
	}

	scanner, sourceLabel, err := buildScanner(*useGit, *useDirs, *useFiles, *listArg, absRoot, *pattern, *depth)
	if err != nil {
		fatalf("%v", err)
	}

	filter, err := buildFilter(*useGit, *filterArg)
	if err != nil {
		fatalf("filter: %v", err)
	}

	cfg := bulk.Config{
		Concurrency: *concurrency,
		Timeout:     time.Duration(*timeoutSec) * time.Second,
		DryRun:      *dryRun,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if !*asJSON {
		fmt.Printf("Scanning %s in %s (depth: %d)...\n", sourceLabel, absRoot, *depth)
	}

	summary, err := bulk.Execute(ctx, scanner, filter, bulk.Command{Template: command}, cfg)
	if err != nil {
		fatalf("execute: %v", err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			fatalf("encode: %v", err)
		}
		return
	}

	printHuman(summary, *filterArg != "")
}

func buildScanner(useGit, useDirs, useFiles bool, listArg, root, pattern string, depth int) (bulk.Scanner, string, error) {
	switch {
	case useGit:
		return bulk.GitRepoScanner(root, depth), "git repos", nil
	case useDirs:
		matchFn := dirMatchFunc(pattern)
		return &bulk.DirScanner{Root: root, Depth: depth, Match: matchFn}, "directories", nil
	case useFiles:
		return &bulk.GlobScanner{Pattern: pattern, Root: root}, "files", nil
	case listArg != "":
		paths := strings.Split(listArg, ",")
		return &bulk.ListScanner{Paths: paths}, "list", nil
	default:
		return nil, "", fmt.Errorf("specify a source: -git, -dirs, -files, or -list")
	}
}

func buildFilter(isGit bool, filterArg string) (bulk.Filter, error) {
	if filterArg == "" {
		return nil, nil
	}
	if isGit {
		f, err := bulk.GitFilter(filterArg)
		if err == nil {
			return f, nil
		}
		// Unknown git keyword — fall back to shell predicate.
	}
	return bulk.ShellFilter(filterArg), nil
}

func dirMatchFunc(pattern string) func(string) bool {
	if pattern == "" {
		return nil
	}
	return func(path string) bool {
		_, err := os.Stat(filepath.Join(path, pattern))
		return err == nil
	}
}

func printHuman(s bulk.Summary, hasFilter bool) {
	if hasFilter {
		fmt.Printf("Found %d items, %d matched filter\n\n", s.TotalCollected, s.MatchedFilter)
	} else {
		fmt.Printf("Found %d items\n\n", s.TotalCollected)
	}

	ok, failed := 0, 0
	for _, r := range s.Results {
		tag := "[ok] "
		if r.Status == "error" {
			tag = "[err]"
			failed++
		} else if r.Status == "skipped" {
			tag = "[dry]"
			ok++
		} else {
			ok++
		}
		fmt.Printf("%-5s %-20s %s\n", tag, r.Item, r.Output)
	}

	fmt.Printf("\n%d ok, %d failed\n", ok, failed)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "bulk: "+format+"\n", args...)
	os.Exit(1)
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: bulk [flags] <command>

Sources (pick one):
  -git              Scan for git repos
  -dirs             Scan for directories
  -files            Scan for files by glob
  -list item1,item2 Explicit list of paths

Options:
  -root string      Root directory (default: .)
  -pattern string   Match pattern (dirs: filename to find; files: glob)
  -filter string    Shell predicate or git filter (ahead/behind/dirty/clean/diverged)
  -depth int        Scan depth (default: 1)
  -j int            Concurrency (default: 8)
  -timeout int      Per-item timeout in seconds (default: 30)
  -dry-run          Collect and filter without executing
  -json             Output as JSON

Template vars in command: {path}, {name}, {dir}, {basename}

Examples:
  bulk -git git status -s
  bulk -git -filter dirty git add -A && git commit -m "wip"
  bulk -dirs -pattern go.mod go build ./...
  bulk -files -pattern "*.go" -root ./internal gofmt -w {path}
`)
}
