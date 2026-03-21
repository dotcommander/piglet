package tool

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

func lsTool(app *ext.App) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "ls",
			Description: "List directory contents with file sizes and types.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string", "description": "Directory path (default: cwd)"},
					"limit": map[string]any{"type": "integer", "description": "Max entries (default 500)"},
				},
			},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			path := stringArg(args, "path", app.CWD())
			path = resolvePath(app.CWD(), path)
			limit := intArg(args, "limit", 500)

			entries, err := os.ReadDir(path)
			if err != nil {
				return textResult(fmt.Sprintf("error: %v", err)), nil
			}

			var b strings.Builder
			count := 0
			for _, e := range entries {
				if count >= limit {
					break
				}
				info, err := e.Info()
				if err != nil {
					continue
				}

				name := e.Name()
				if e.IsDir() {
					name += "/"
				}

				fmt.Fprintf(&b, "%s\t%s\t%s\n", info.Mode(), formatSize(info.Size()), name)
				count++
			}

			if count == 0 {
				return textResult("(empty directory)"), nil
			}
			if count >= limit {
				fmt.Fprintf(&b, "\n... (%d more entries, use limit to see more)", len(entries)-limit)
			}
			return textResult(b.String()), nil
		},
		PromptHint:     "List directory contents",
		BackgroundSafe: true,
	}
}
