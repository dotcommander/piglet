// webfetch — standalone CLI for web fetch and search.
// Usage: webfetch [flags] <url>
//
//	webfetch search [flags] <query>
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dotcommander/piglet/extensions/webfetch"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  webfetch [flags] <url>           fetch a URL as clean markdown
  webfetch search [flags] <query>  search the web

Flags (fetch):
  -raw          Fetch URL directly without reader provider
  -json         Output as JSON

Flags (search):
  -limit int    Max search results (default 5)
  -json         Output as JSON

Examples:
  webfetch https://example.com
  webfetch -raw https://api.example.com/data.json
  webfetch search "golang error handling best practices"
  webfetch search -limit 10 -json "rust vs go"
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	if os.Args[1] == "search" {
		runSearch(os.Args[2:])
	} else {
		runFetch(os.Args[1:])
	}
}

func runFetch(args []string) {
	fs := flag.NewFlagSet("webfetch", flag.ContinueOnError)
	fs.Usage = usage

	raw := fs.Bool("raw", false, "fetch URL directly without reader provider")
	asJSON := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: URL required")
		usage()
		os.Exit(1)
	}

	url := strings.Join(fs.Args(), " ")

	client := loadClient()

	result, err := client.Fetch(context.Background(), url, *raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]string{"url": url, "content": result}); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Print(result)
}

func runSearch(args []string) {
	fs := flag.NewFlagSet("webfetch search", flag.ContinueOnError)
	fs.Usage = usage

	limit := fs.Int("limit", 5, "max search results")
	asJSON := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: query required")
		usage()
		os.Exit(1)
	}

	query := strings.Join(fs.Args(), " ")

	client := loadClient()

	results, err := client.Search(context.Background(), query, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Print(webfetch.FormatResults(results))
}

func loadClient() *webfetch.Client {
	cfg, err := webfetch.LoadConfig()
	if err != nil {
		return webfetch.Default()
	}
	return webfetch.NewWithConfig(cfg)
}
