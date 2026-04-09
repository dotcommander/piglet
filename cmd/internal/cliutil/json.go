// Package cliutil provides shared helpers for standalone CLI commands.
package cliutil

import (
	"encoding/json"
	"fmt"
	"os"
)

// PrintJSON writes v as indented JSON to stdout. Exits on encode failure.
func PrintJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "error: json encode: %v\n", err)
		os.Exit(1)
	}
}
