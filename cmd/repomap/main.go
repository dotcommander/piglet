// repomap — standalone CLI for the repomap package.
// Usage: repomap [flags] [directory]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dotcommander/piglet/extensions/repomap"
)

func main() {
	tokens := flag.Int("tokens", 2048, "Token budget")
	format := flag.String("format", "compact", "Output format: compact, verbose, detail, lines")
	asJSON := flag.Bool("json", false, "Output as JSON (file list with symbols)")
	flag.Parse()

	dir := "."
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}

	cfg := repomap.Config{
		MaxTokens:      *tokens,
		MaxTokensNoCtx: *tokens,
	}
	m := repomap.New(dir, cfg)

	if err := m.Build(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "repomap: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		if err := printJSON(m); err != nil {
			fmt.Fprintf(os.Stderr, "repomap: %v\n", err)
			os.Exit(1)
		}
		return
	}

	var out string
	switch *format {
	case "verbose":
		out = m.StringVerbose()
	case "detail":
		out = m.StringDetail()
	case "lines":
		out = m.StringLines()
	default:
		out = m.String()
	}
	fmt.Print(out)
}

// printJSON emits the verbose map as a JSON array of lines.
func printJSON(m *repomap.Map) error {
	verbose := m.StringVerbose()
	lines := strings.Split(strings.TrimRight(verbose, "\n"), "\n")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(lines)
}
