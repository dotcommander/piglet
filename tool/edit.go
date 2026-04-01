package tool

import (
	"context"
	"fmt"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"os"
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
			path, errResult := requirePath(args, app.CWD())
			if errResult != nil {
				return errResult, nil
			}
			oldText, _ := args["old_text"].(string)
			newText, _ := args["new_text"].(string)
			if oldText == "" {
				return textResult("error: old_text is required and cannot be empty"), nil
			}

			// TOCTOU staleness check — catch concurrent modifications.
			if msg := tracker.CheckStale(path); msg != "" {
				return textResult("error: " + msg), nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return textResult(fmt.Sprintf("error reading file: %v", err)), nil
			}

			content := string(data)

			// Find with quote normalization fallback.
			actual, count := findWithQuoteNormalization(content, oldText)
			if count == 0 {
				return textResult("error: old_text not found in file"), nil
			}
			if count > 1 {
				return textResult(fmt.Sprintf("error: old_text found %d times, must be unique. Add more context to make it unique.", count)), nil
			}

			// If we matched via normalization, apply curly quotes to the replacement.
			replacement := newText
			if actual != oldText {
				replacement = applyCurlyQuotes(actual, newText)
			}

			updated := strings.Replace(content, actual, replacement, 1)

			// Snapshot for undo
			snapshotFile(path)

			if err := atomicWrite(path, []byte(updated)); err != nil {
				return textResult(fmt.Sprintf("error writing file: %v", err)), nil
			}

			// Re-record mtime so subsequent edits don't trigger false staleness.
			if info, err := os.Stat(path); err == nil {
				tracker.RecordRead(path, info.ModTime())
			}

			return textResult(fmt.Sprintf("edited %s", path)), nil
		},
		PromptHint:   "Edit files with search/replace",
		PromptGuides: []string{"old_text must match exactly one occurrence in the file"},
	}
}
