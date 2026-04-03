package clipboard

import (
	"context"

	sdk "github.com/dotcommander/piglet/sdk"
)

// Register adds clipboard's tool and shortcut to the extension.
func Register(e *sdk.Extension) {
	e.RegisterTool(sdk.ToolDef{
		Name:        "clipboard_read",
		Description: "Read an image from the system clipboard. Returns the image as base64 data. Only works on macOS.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		PromptHint:  "Read images from system clipboard (macOS)",
		Execute: func(_ context.Context, _ map[string]any) (*sdk.ToolResult, error) {
			img, err := ReadImage()
			if err != nil {
				return sdk.ErrorResult("error: " + err.Error()), nil
			}
			return &sdk.ToolResult{
				Content: []sdk.ContentBlock{{Type: "image", Data: img.Data, Mime: img.MimeType}},
			}, nil
		},
	})

	e.RegisterShortcut(sdk.ShortcutDef{
		Key:         "ctrl+v",
		Description: "Attach clipboard image",
		Handler: func(_ context.Context) (*sdk.Action, error) {
			img, err := ReadImage()
			if err != nil {
				return sdk.ActionNotify("No image: " + err.Error()), nil
			}
			return sdk.ActionAttachImage(img.Data, img.MimeType), nil
		},
	})
}
