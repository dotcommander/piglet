package tool

import (
	"context"
	"fmt"
	"os"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"strings"
)

func editTool(app *ext.App) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "edit",
			Description: "Edit a file by replacing exact text. The old_text must match exactly (including whitespace).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string", "description": "Absolute file path"},
					"old_text": map[string]any{"type": "string", "description": "Exact text to find and replace"},
					"new_text": map[string]any{"type": "string", "description": "Replacement text"},
				},
				"required": []string{"path", "old_text", "new_text"},
			},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			path, _ := args["path"].(string)
			oldText, _ := args["old_text"].(string)
			newText, _ := args["new_text"].(string)

			if path == "" {
				return textResult("error: path is required"), nil
			}
			path = resolvePath(app.CWD(), path)

			data, err := os.ReadFile(path)
			if err != nil {
				return textResult(fmt.Sprintf("error reading file: %v", err)), nil
			}

			content := string(data)

			// Count occurrences
			count := strings.Count(content, oldText)
			if count == 0 {
				return textResult("error: old_text not found in file"), nil
			}
			if count > 1 {
				return textResult(fmt.Sprintf("error: old_text found %d times, must be unique. Add more context to make it unique.", count)), nil
			}

			// Replace
			updated := strings.Replace(content, oldText, newText, 1)

			// Atomic write
			tmp := path + ".piglet-tmp"
			if err := os.WriteFile(tmp, []byte(updated), 0644); err != nil {
				return textResult(fmt.Sprintf("error writing file: %v", err)), nil
			}
			if err := os.Rename(tmp, path); err != nil {
				os.Remove(tmp)
				return textResult(fmt.Sprintf("error renaming file: %v", err)), nil
			}

			return textResult(fmt.Sprintf("edited %s", path)), nil
		},
		PromptHint:   "Edit files with search/replace",
		PromptGuides: []string{"old_text must match exactly one occurrence in the file"},
	}
}
