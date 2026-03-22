// Bulk extension binary. Runs a shell command across many items in parallel.
// Sources: git_repos, dirs, files, list. Filters: shell predicates or git conditions.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/bulk"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

func main() {
	e := sdk.New("bulk", "0.1.0")

	e.RegisterPromptSection(sdk.PromptSectionDef{
		Title:   "Bulk Operations",
		Content: "Use bulk to run a command across multiple items in parallel. Sources: git_repos (scan for .git/), dirs (match by name/file), files (glob), list (explicit paths). Filters: shell predicates (exit 0 = keep) or git-specific (ahead/behind/dirty/clean/diverged for git_repos). Template vars: {path}, {name}, {dir}, {basename}. Dry-run by default for mutating commands (push, rm, delete, clean, reset).",
		Order:   80,
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "bulk",
		Description: "Run a shell command across multiple items (directories, files, git repos) in parallel. Discovers items, optionally filters, executes command, returns structured results.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to run on each item. Template vars: {path}, {name}, {dir}, {basename}. Runs in item's directory for dir-based sources.",
				},
				"source": map[string]any{
					"type":        "string",
					"enum":        []string{"git_repos", "dirs", "files", "list"},
					"description": "How to collect items. git_repos: dirs with .git/. dirs: subdirs matching pattern. files: glob match. list: explicit paths.",
				},
				"directory": map[string]any{
					"type":        "string",
					"description": "Root directory to scan (default: cwd). Used by git_repos, dirs, files.",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "For dirs: filename/dir to match (e.g. 'go.mod', 'Makefile'). For files: glob pattern (e.g. '*.go').",
				},
				"items": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Explicit paths for list source.",
				},
				"filter": map[string]any{
					"type":        "string",
					"description": "Shell predicate — keeps items where exit 0. For git_repos, also accepts: ahead, behind, dirty, clean, diverged.",
				},
				"depth": map[string]any{
					"type":        "integer",
					"description": "Scan depth (default: 1)",
				},
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Collect and filter without executing. Auto-true for mutating commands (push, rm, delete, clean, reset).",
				},
				"concurrency": map[string]any{
					"type":        "integer",
					"description": "Max parallel executions (default: 8)",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Per-item timeout in seconds (default: 30)",
				},
			},
			"required": []string{"command", "source"},
		},
		Execute: executeBulk,
	})

	e.Run()
}

// executeBulk is the tool handler. Parses args and wires to the bulk package.
func executeBulk(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
	command, _ := args["command"].(string)
	source, _ := args["source"].(string)

	directory := cwdOrArg(args)
	pattern, _ := args["pattern"].(string)
	filterArg, _ := args["filter"].(string)
	depth := intArg(args, "depth", 1)
	concurrency := intArg(args, "concurrency", 8)
	timeoutSec := intArg(args, "timeout", 30)

	var items []string
	if raw, ok := args["items"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				items = append(items, s)
			}
		}
	}

	dryRun := dryRunArg(args, command)

	scanner, err := buildScanner(source, directory, pattern, depth, items)
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("invalid source: %s", err)), nil
	}

	filter, err := buildFilter(source, filterArg)
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("invalid filter: %s", err)), nil
	}

	cfg := bulk.Config{
		Concurrency: concurrency,
		Timeout:     time.Duration(timeoutSec) * time.Second,
		DryRun:      dryRun,
	}

	summary, err := bulk.Execute(ctx, scanner, filter, bulk.Command{Template: command}, cfg)
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("bulk execute: %s", err)), nil
	}

	data, err := json.Marshal(summary)
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("marshal result: %s", err)), nil
	}

	return sdk.TextResult(string(data)), nil
}

// buildScanner creates the appropriate Scanner for the given source type.
func buildScanner(source, directory, pattern string, depth int, items []string) (bulk.Scanner, error) {
	switch source {
	case bulk.SourceGitRepos:
		return bulk.GitRepoScanner(directory, depth), nil
	case bulk.SourceDirs:
		matchFn := dirMatchFunc(pattern)
		return &bulk.DirScanner{Root: directory, Depth: depth, Match: matchFn}, nil
	case bulk.SourceFiles:
		return &bulk.GlobScanner{Pattern: pattern, Root: directory}, nil
	case bulk.SourceList:
		return &bulk.ListScanner{Paths: items}, nil
	default:
		return nil, fmt.Errorf("unknown source %q", source)
	}
}

// dirMatchFunc returns a match function that checks if pattern exists inside a dir.
// Nil is returned when pattern is empty (match all dirs).
func dirMatchFunc(pattern string) func(string) bool {
	if pattern == "" {
		return nil
	}
	return func(path string) bool {
		_, err := os.Stat(filepath.Join(path, pattern))
		return err == nil
	}
}

// buildFilter creates the appropriate Filter for the given source and filter string.
func buildFilter(source, filterArg string) (bulk.Filter, error) {
	if filterArg == "" {
		return nil, nil
	}
	if source == "git_repos" {
		f, err := bulk.GitFilter(filterArg)
		if err == nil {
			return f, nil
		}
		// Not a known git filter keyword — treat as shell predicate.
	}
	return bulk.ShellFilter(filterArg), nil
}

// cwdOrArg returns the directory arg if set, otherwise the process working directory.
func cwdOrArg(args map[string]any) string {
	if d, ok := args["directory"].(string); ok && d != "" {
		return d
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// intArg extracts an integer arg, returning def if missing or zero.
func intArg(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		if int(v) > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	}
	return def
}

// dryRunArg returns the dry_run value, defaulting to true for mutating commands.
func dryRunArg(args map[string]any, command string) bool {
	if v, ok := args["dry_run"].(bool); ok {
		return v
	}
	return isMutating(command)
}

// isMutating returns true if the command contains a potentially destructive keyword.
func isMutating(command string) bool {
	for _, kw := range []string{"push", "rm ", "delete", "clean", "reset", "--force"} {
		if strings.Contains(command, kw) {
			return true
		}
	}
	return false
}
