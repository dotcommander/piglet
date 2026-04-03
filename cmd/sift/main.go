// sift — compress text output by collapsing blanks, repeats, and truncating.
// Usage: sift [flags] < input
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dotcommander/piglet/extensions/sift"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: sift [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Reads stdin, compresses output, writes to stdout.\n\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  cat large-output.txt | sift\n")
	fmt.Fprintf(os.Stderr, "  git diff | sift -threshold 2048\n")
	fmt.Fprintf(os.Stderr, "  make build 2>&1 | sift -max-size 16384\n")
}

func main() {
	threshold := flag.Int("threshold", 0, "Minimum size before compression kicks in (default: 4096)")
	maxSize := flag.Int("max-size", 0, "Maximum output size (default: 32768)")
	noBlanks := flag.Bool("no-collapse-blanks", false, "Disable blank line collapsing")
	noRepeats := flag.Bool("no-collapse-repeats", false, "Disable repeated line collapsing")
	flag.Usage = usage
	flag.Parse()

	cfg := sift.LoadConfig()

	if *threshold != 0 {
		cfg.SizeThreshold = *threshold
	}
	if *maxSize != 0 {
		cfg.MaxSize = *maxSize
	}
	if *noBlanks {
		cfg.Compression.CollapseBlankLines = 0
	}
	if *noRepeats {
		cfg.Compression.CollapseRepeatedLines = 0
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sift: %v\n", err)
		os.Exit(1)
	}

	if len(input) == 0 {
		os.Exit(0)
	}

	fmt.Print(sift.Compress(string(input), cfg))
}
