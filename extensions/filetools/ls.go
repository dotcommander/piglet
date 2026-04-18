package filetools

import (
	"context"
	"fmt"
	"os"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

func registerLs(e *sdk.Extension) {
	e.RegisterTool(sdk.ToolDef{
		Name:        "ls",
		Description: "List directory contents with file sizes and types.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string", "description": "Directory path (default: cwd)"},
				"limit": map[string]any{"type": "integer", "description": "Max entries (default 500)"},
			},
		},
		PromptHint: "List directory contents",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			cwd := e.CWD()
			path := stringArg(args, "path", cwd)
			path = resolvePath(cwd, path)
			limit := intArg(args, "limit", 500)

			entries, err := os.ReadDir(path)
			if err != nil {
				return textResult(fmt.Sprintf("error: %v", err)), nil
			}

			var b strings.Builder
			count := 0
			for i, entry := range entries {
				if count >= limit {
					fmt.Fprintf(&b, "\n... (%d more entries, use limit to see more)", len(entries)-i)
					break
				}
				info, err := entry.Info()
				if err != nil {
					continue
				}

				name := entry.Name()
				if entry.IsDir() {
					name += "/"
				}

				fmt.Fprintf(&b, "%s\t%s\t%s\n", info.Mode(), FormatSize(info.Size()), name)
				count++
			}

			if count == 0 {
				return textResult("(empty directory)"), nil
			}
			return textResult(b.String()), nil
		},
	})
}
