package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

var errLimitReached = errors.New("limit reached")

func findTool(app *ext.App) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "find",
			Description: "Find files matching a glob pattern. Returns relative file paths.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. \"**/*.go\", \"*.txt\")"},
					"path":    map[string]any{"type": "string", "description": "Directory to search (default: cwd)"},
					"limit":   map[string]any{"type": "integer", "description": "Max results (default 1000)"},
				},
				"required": []string{"pattern"},
			},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return textResult("error: pattern is required"), nil
			}

			searchPath := stringArg(args, "path", app.CWD())
			searchPath = resolvePath(app.CWD(), searchPath)
			limit := intArg(args, "limit", 1000)

			info, err := os.Stat(searchPath)
			if err != nil {
				return textResult(fmt.Sprintf("error: %v", err)), nil
			}
			if !info.IsDir() {
				return textResult("error: path must be a directory"), nil
			}

			var results []string
			fsys := filteredFS{base: os.DirFS(searchPath), skip: shouldSkipDir}
			_ = doublestar.GlobWalk(fsys, pattern, func(path string, d os.DirEntry) error {
				if d.IsDir() {
					return nil
				}
				if len(results) >= limit {
					return errLimitReached
				}
				results = append(results, path)
				return nil
			}, doublestar.WithNoFollow())

			if len(results) == 0 {
				return textResult("no files found matching pattern"), nil
			}

			var b strings.Builder
			for _, r := range results {
				b.WriteString(r)
				b.WriteString("\n")
			}
			if len(results) >= limit {
				fmt.Fprintf(&b, "\n... (limit of %d results reached)", limit)
			}
			return textResult(b.String()), nil
		},
		PromptHint:     "Find files by glob pattern",
		BackgroundSafe: true,
	}
}
