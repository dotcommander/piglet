package tokengate

import (
	_ "embed"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed defaults/prompt.md
var defaultPrompt string

// LoadPrompt reads the token gate prompt from the extension config directory.
func LoadPrompt() string {
	return xdg.LoadOrCreateExt("tokengate", "prompt.md", strings.TrimSpace(defaultPrompt))
}
