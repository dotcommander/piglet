// Package filetools provides the grep, find, and ls tools as an external extension
// for piglet. These tools are bundled into pack-code via filetools.Register.
package filetools

import (
	"fmt"
	"path/filepath"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

// ---------------------------------------------------------------------------
// Tool result helpers
// ---------------------------------------------------------------------------

func textResult(text string) *sdk.ToolResult {
	return &sdk.ToolResult{
		Content: []sdk.ContentBlock{{Type: "text", Text: text}},
	}
}

// ---------------------------------------------------------------------------
// Argument extraction (JSON numbers decode as float64)
// ---------------------------------------------------------------------------

func stringArg(args map[string]any, key, fallback string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

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

func boolArg(args map[string]any, key string, fallback bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return fallback
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

func resolvePath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

// ---------------------------------------------------------------------------
// Directory filtering — shared by find and grep (via walk)
// ---------------------------------------------------------------------------

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".next", "__pycache__", ".cache", "dist", "build":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

// ---------------------------------------------------------------------------
// Display formatting
// ---------------------------------------------------------------------------

// FormatSize converts a byte count to a human-readable string.
func FormatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
