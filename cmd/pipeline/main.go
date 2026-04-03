// pipeline — standalone CLI for running YAML pipeline files.
//
// Usage:
//
//	pipeline [flags] <file.yaml>
//	pipeline list <directory>
//
// Flags:
//
//	-dry-run          Preview without executing
//	-param key=value  Parameter override (repeatable)
//	-json             Output as JSON instead of human-readable
//	-q                Quiet mode — only show errors and final status
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dotcommander/piglet/extensions/pipeline"
)

// paramList is a repeatable -param flag.
type paramList []string

func (p *paramList) String() string { return strings.Join(*p, ", ") }
func (p *paramList) Set(v string) error {
	*p = append(*p, v)
	return nil
}

func main() {
	fs := flag.NewFlagSet("pipeline", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "preview without executing")
	asJSON := fs.Bool("json", false, "output as JSON")
	quiet := fs.Bool("q", false, "quiet — errors and final status only")

	var params paramList
	fs.Var(&params, "param", "parameter override: key=value (repeatable)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pipeline [flags] <file.yaml>")
		fmt.Fprintln(os.Stderr, "       pipeline list <directory>")
		os.Exit(2)
	}

	if args[0] == "list" {
		runList(args)
		return
	}

	runPipeline(args[0], params, *dryRun, *asJSON, *quiet)
}

func runList(args []string) {
	dir := "."
	if len(args) >= 2 {
		dir = args[1]
	}

	pipes, err := pipeline.LoadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
	if len(pipes) == 0 {
		fmt.Fprintf(os.Stderr, "no pipelines found in %s\n", dir)
		return
	}
	for _, p := range pipes {
		if p.Description != "" {
			fmt.Printf("%-30s %s\n", p.Name, p.Description)
		} else {
			fmt.Println(p.Name)
		}
	}
}

func runPipeline(path string, rawParams paramList, dryRun, asJSON, quiet bool) {
	p, err := pipeline.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	overrides := parseParams(rawParams)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var result *pipeline.PipelineResult
	if dryRun {
		result, err = pipeline.DryRun(p, overrides)
	} else {
		result, err = pipeline.Run(ctx, p, overrides)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
	} else {
		printResult(result, quiet)
	}

	if result.Status == pipeline.StatusError {
		os.Exit(1)
	}
}

func parseParams(raw []string) map[string]string {
	out := make(map[string]string, len(raw))
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}

func printResult(result *pipeline.PipelineResult, quiet bool) {
	if !quiet {
		fmt.Printf("Pipeline: %s\n", result.Name)
	}

	for _, sr := range result.Steps {
		icon := "+"
		switch sr.Status {
		case pipeline.StatusError:
			icon = "x"
		case pipeline.StatusSkipped:
			icon = "-"
		}

		if quiet && sr.Status != pipeline.StatusError {
			continue
		}

		fmt.Printf("[%s] %s (%dms)\n", icon, sr.Name, sr.DurationMS)

		if !quiet && sr.Output != "" {
			for _, line := range strings.Split(strings.TrimRight(sr.Output, "\n"), "\n") {
				fmt.Printf("    %s\n", line)
			}
		}

		if sr.Error != "" {
			fmt.Printf("    error: %s\n", sr.Error)
		}
	}

	fmt.Printf("\nResult: %s — %s (%dms)\n", result.Status, result.Message, result.DurationMS)
}
