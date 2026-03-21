package tool

import (
	"os"
	"path/filepath"

	"github.com/dotcommander/piglet/core"
)

// textResult creates a simple text ToolResult.
func textResult(text string) *core.ToolResult {
	return &core.ToolResult{
		Content: []core.ContentBlock{core.TextContent{Text: text}},
	}
}

// resolvePath resolves a path relative to cwd.
func resolvePath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

// stringArg extracts a string argument with a default.
func stringArg(args map[string]any, key, fallback string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

// intArg extracts an int argument with a default.
// Handles both int and float64 (JSON numbers decode as float64).
func intArg(args map[string]any, key string, fallback int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return fallback
	}
}

// boolArg extracts a bool argument with a default.
func boolArg(args map[string]any, key string, fallback bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return fallback
}

// requirePath extracts and resolves a path argument.
// Returns an error result if the path is empty.
func requirePath(args map[string]any, cwd string) (string, *core.ToolResult) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", textResult("error: path is required")
	}
	return resolvePath(cwd, path), nil
}

// atomicWrite writes data to path via a temp file and rename.
func atomicWrite(path string, data []byte) error {
	tmp := path + ".piglet-tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
