package tool

import (
	"context"
	"fmt"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"os"
	"path/filepath"
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

			// TOCTOU staleness check — catch concurrent modifications.
			if msg := tracker.CheckStale(path); msg != "" {
				return textResult("error: " + msg), nil
			}

			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return textResult(fmt.Sprintf("error creating directory: %v", err)), nil
			}

			// Snapshot for undo
			snapshotFile(path)

			if err := atomicWrite(path, []byte(content)); err != nil {
				return textResult(fmt.Sprintf("error writing file: %v", err)), nil
			}

			// Re-record mtime so subsequent writes don't trigger false staleness.
			if info, err := os.Stat(path); err == nil {
				tracker.RecordRead(path, info.ModTime())
			}

			return textResult(fmt.Sprintf("wrote %d bytes to %s", len(content), path)), nil
		},
		PromptHint: "Write content to a file",
	}
}
