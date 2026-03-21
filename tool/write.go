package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

func writeTool(app *ext.App) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "write",
			Description: "Write content to a file. Creates parent directories if needed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Absolute file path"},
					"content": map[string]any{"type": "string", "description": "File content to write"},
				},
				"required": []string{"path", "content"},
			},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			path, errResult := requirePath(args, app.CWD())
			if errResult != nil {
				return errResult, nil
			}
			content, _ := args["content"].(string)

			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return textResult(fmt.Sprintf("error creating directory: %v", err)), nil
			}

			// Snapshot for undo
			snapshotFile(path)

			if err := atomicWrite(path, []byte(content)); err != nil {
				return textResult(fmt.Sprintf("error writing file: %v", err)), nil
			}

			return textResult(fmt.Sprintf("wrote %d bytes to %s", len(content), path)), nil
		},
		PromptHint: "Write content to a file",
	}
}
