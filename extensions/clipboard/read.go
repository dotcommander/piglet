package clipboard

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// ImageData holds base64-encoded image data.
type ImageData struct {
	Data     string
	MimeType string
}

// ReadImage reads an image from the macOS clipboard.
// Returns the image as base64-encoded ImageData, or an error if no image is available.
func ReadImage() (*ImageData, error) {
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

	clipClass := "«class PNGf»"
	if mime == "image/jpeg" {
		clipClass = "«class JPEG»"
	}

	script := fmt.Sprintf(
		"set imageData to the clipboard as %s\n"+
			"set theFile to (open for access POSIX file \"/dev/stdout\" with write permission)\n"+
			"write imageData to theFile\n"+
			"close access theFile", clipClass)
	pbCmd := exec.Command("osascript", "-e", script)
	data, err := pbCmd.Output()
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("failed to read image from clipboard")
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return &ImageData{
		Data:     encoded,
		MimeType: mime,
	}, nil
}
