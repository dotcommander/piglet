package clipboard

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"github.com/dotcommander/piglet/core"
)

// ReadImage reads an image from the macOS clipboard.
// Returns the image as base64-encoded ImageContent, or an error if no image is available.
func ReadImage() (*core.ImageContent, error) {
	cmd := exec.Command("osascript", "-e", "the clipboard info")
	info, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("clipboard not available")
	}

	infoStr := string(info)
	var mime string
	switch {
	case strings.Contains(infoStr, "PNGf"):
		mime = "image/png"
	case strings.Contains(infoStr, "JPEG"):
		mime = "image/jpeg"
	default:
		return nil, fmt.Errorf("no image in clipboard")
	}

	pbCmd := exec.Command("osascript", "-e",
		`set imageData to the clipboard as «class PNGf»
set theFile to (open for access POSIX file "/dev/stdout" with write permission)
write imageData to theFile
close access theFile`)
	data, err := pbCmd.Output()
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("failed to read image from clipboard")
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return &core.ImageContent{
		Data:     encoded,
		MimeType: mime,
	}, nil
}
