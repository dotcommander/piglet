package clipboard

import (
	"context"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// Register sets up the clipboard_read tool and Ctrl+I shortcut.
func Register(app *ext.App) {
	// Keyboard shortcut: Ctrl+I attaches clipboard image to next message
	app.RegisterShortcut(&ext.Shortcut{
		Key:         "ctrl+v",
		Description: "Attach clipboard image",
		Handler: func(a *ext.App) (ext.Action, error) {
			return ext.ActionRunAsync{Fn: func() ext.Action {
				img, err := ReadImage()
				if err != nil {
					return ext.ActionNotify{Message: "No image: " + err.Error()}
				}
				return ext.ActionAttachImage{Image: img}
			}}, nil
		},
	})
	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "clipboard_read",
			Description: "Read an image from the system clipboard. Returns the image as base64 data. Only works on macOS.",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
		Execute: func(_ context.Context, _ string, _ map[string]any) (*core.ToolResult, error) {
			img, err := ReadImage()
			if err != nil {
				return ext.TextResult("error: " + err.Error()), nil
			}
			return &core.ToolResult{
				Content: []core.ContentBlock{*img},
			}, nil
		},
		PromptHint: "Read images from system clipboard (macOS)",
	})
}
