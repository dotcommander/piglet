package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"strings"
)

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
			filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() && shouldSkipDir(d.Name()) {
					return filepath.SkipDir
				}
				if d.IsDir() {
					return nil
				}
				if len(results) >= limit {
					return filepath.SkipAll
				}

				rel, _ := filepath.Rel(searchPath, path)
				matched, _ := filepath.Match(pattern, d.Name())
				if !matched && strings.Contains(pattern, "**") {
					matched, _ = filepath.Match(strings.TrimPrefix(pattern, "**/"), d.Name())
				}
				if !matched {
					matched, _ = filepath.Match(pattern, rel)
				}
				if matched {
					results = append(results, rel)
				}
				return nil
			})

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
