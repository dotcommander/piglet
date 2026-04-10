package coordinator

import (
	_ "embed"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed defaults/prompt.md
var defaultPlanPrompt string

func LoadPlanPrompt() string {
	return xdg.LoadOrCreateExt(extName, "prompt.md", strings.TrimSpace(defaultPlanPrompt))
}
