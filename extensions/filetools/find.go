package filetools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	sdk "github.com/dotcommander/piglet/sdk"
)

func registerFind(e *sdk.Extension) {
	e.RegisterTool(sdk.ToolDef{
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
		PromptHint: "Find files by glob pattern",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return textResult("error: pattern is required"), nil
			}

			cwd := e.CWD()
			searchPath := stringArg(args, "path", cwd)
			searchPath = resolvePath(cwd, searchPath)
			limit := intArg(args, "limit", 1000)

			info, err := os.Stat(searchPath)
			if err != nil {
				return textResult(fmt.Sprintf("error: %v", err)), nil
			}
			if !info.IsDir() {
				return textResult("error: path must be a directory"), nil
			}

			var results []string
			walkErr := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
				if err != nil && path == searchPath {
					return err // propagate root-level errors (e.g., permission denied)
				}
				if err != nil {
					return nil
				}
				if d.IsDir() {
					if shouldSkipDir(d.Name()) {
						return filepath.SkipDir
					}
					return nil
				}
				rel, _ := filepath.Rel(searchPath, path)
				matched, _ := doublestar.Match(pattern, rel)
				if !matched {
					return nil
				}
				if len(results) >= limit {
					return filepath.SkipAll
				}
				results = append(results, rel)
				return nil
			})

			if len(results) == 0 {
				if walkErr != nil {
					return textResult(fmt.Sprintf("error: %v", walkErr)), nil
				}
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
	})
}
