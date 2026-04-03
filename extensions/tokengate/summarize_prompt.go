package tokengate

import (
	_ "embed"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed defaults/summarize-prompt.md
var defaultSummarizePrompt string

func LoadSummarizePrompt() string {
	return xdg.LoadOrCreateExt("tokengate", "summarize-prompt.md", strings.TrimSpace(defaultSummarizePrompt))
}
