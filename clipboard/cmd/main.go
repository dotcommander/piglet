// Clipboard extension binary. Reads images from the system clipboard.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"

	"github.com/dotcommander/piglet/clipboard"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

func main() {
	e := sdk.New("clipboard", "0.1.0")

	// Tool: read image from clipboard
	e.RegisterTool(sdk.ToolDef{
		Name:        "clipboard_read",
		Description: "Read an image from the system clipboard. Returns the image as base64 data. Only works on macOS.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		PromptHint:  "Read images from system clipboard (macOS)",
		Execute: func(_ context.Context, _ map[string]any) (*sdk.ToolResult, error) {
			img, err := clipboard.ReadImage()
			if err != nil {
				return sdk.ErrorResult("error: " + err.Error()), nil
			}
			return &sdk.ToolResult{
				Content: []sdk.ContentBlock{{Type: "image", Data: img.Data, Mime: img.MimeType}},
			}, nil
		},
	})

	// Shortcut: Ctrl+V attaches clipboard image
	e.RegisterShortcut(sdk.ShortcutDef{
		Key:         "ctrl+v",
		Description: "Attach clipboard image",
		Handler: func(_ context.Context) (*sdk.Action, error) {
			img, err := clipboard.ReadImage()
			if err != nil {
				return sdk.ActionNotify("No image: " + err.Error()), nil
			}
			return sdk.ActionAttachImage(img.Data, img.MimeType), nil
		},
	})

	e.Run()
}
