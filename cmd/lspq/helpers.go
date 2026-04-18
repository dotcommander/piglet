package main

import (
	"path/filepath"
	"strconv"

	"github.com/dotcommander/piglet/extensions/lsp"
)

// resolveURIToRel converts an LSP URI to a relative path from cwd.
func resolveURIToRel(uri, cwd string) string {
	path := uriToPath(uri)
	if cwd == "" {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	return rel
}

// uriToPath converts a file:// URI to a filesystem path.
func uriToPath(uri string) string {
	const prefix = "file://"
	if len(uri) > len(prefix) && uri[:len(prefix)] == prefix {
		return uri[len(prefix):]
	}
	return uri
}

// resolveCol determines the 0-based column from remaining positional args or the -col flag.
func resolveCol(file string, line int, rest []string, colFlag int) (int, error) {
	if len(rest) > 0 {
		sym := rest[0]
		if _, err := strconv.Atoi(sym); err != nil {
			// Non-numeric: treat as symbol name, auto-detect column.
			return lsp.FindSymbolColumn(file, line, sym)
		}
	}
	if colFlag > 0 {
		return colFlag - 1, nil // convert 1-based to 0-based
	}
	return 0, nil
}

func resolveFile(file, cwd string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(cwd, file)
}
