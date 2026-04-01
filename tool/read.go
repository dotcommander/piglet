package tool

import (
	"context"
	"fmt"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"os"
	"strings"
)

func readTool(app *ext.App, cfg ToolConfig) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "read",
			Description: "Read file contents with line numbers. Use offset/limit for large files.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Absolute file path"},
					"offset": map[string]any{"type": "integer", "description": "Start line (1-indexed, optional)"},
					"limit":  map[string]any{"type": "integer", "description": "Max lines to read (optional, default 2000)"},
				},
				"required": []string{"path"},
			},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			path, errResult := requirePath(args, app.CWD())
			if errResult != nil {
				return errResult, nil
			}

			info, err := os.Stat(path)
			if err != nil {
				return textResult(fmt.Sprintf("error: %v", err)), nil
			}
			const maxReadSize = 50 << 20 // 50 MB
			if !info.Mode().IsRegular() {
				return textResult("error: not a regular file"), nil
			}
			if info.Size() > maxReadSize {
				return textResult(fmt.Sprintf("error: file too large (%s, max 50MB)", FormatSize(info.Size()))), nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return textResult(fmt.Sprintf("error: %v", err)), nil
			}

			// Track mtime for TOCTOU staleness detection in edit/write tools.
			tracker.RecordRead(path, info.ModTime())

			lines := strings.Split(string(data), "\n")

			offset := intArg(args, "offset", 1)
			if offset < 1 {
				offset = 1
			}
			limit := intArg(args, "limit", cfg.readLimit())
			if limit < 1 {
				limit = cfg.readLimit()
			}

			start := offset - 1
			if start >= len(lines) {
				return textResult(fmt.Sprintf("file has %d lines, offset %d is past end", len(lines), offset)), nil
			}

			end := start + limit
			if end > len(lines) {
				end = len(lines)
			}

			var b strings.Builder
			maxWidth := len(fmt.Sprintf("%d", end))
			for i := start; i < end; i++ {
				fmt.Fprintf(&b, "%*d\t%s\n", maxWidth, i+1, lines[i])
			}

			if end < len(lines) {
				fmt.Fprintf(&b, "\n... (%d more lines, use offset/limit to read more)", len(lines)-end)
			}

			return textResult(b.String()), nil
		},
		PromptHint:     "Read file contents with line numbers",
		PromptGuides:   []string{"Use offset/limit for files >2000 lines"},
		BackgroundSafe: true,
	}
}
