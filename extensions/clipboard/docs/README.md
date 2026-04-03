# Clipboard

Reads images from the macOS system clipboard and makes them available to Claude.

## Capabilities

| Capability | Name | Description |
|------------|------|-------------|
| tool | `clipboard_read` | Read image from clipboard as base64 |
| shortcut | `Ctrl+V` | Paste clipboard image into conversation |

## How It Works

Uses macOS `osascript` (AppleScript) to interact with the system clipboard:

1. Checks clipboard type via `clipboard info` to detect PNG or JPEG
2. Reads raw image bytes and encodes to base64
3. Returns the image with auto-detected MIME type

## Platform

macOS only — requires `osascript`.

## Failure Behavior

Returns a descriptive error if no image is available in the clipboard. The keyboard shortcut shows an inline notification on failure.
