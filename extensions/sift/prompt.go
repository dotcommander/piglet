package sift

import (
	_ "embed"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed defaults/prompt.md
var defaultPrompt string

func LoadPrompt() string {
	return xdg.LoadOrCreateExt("sift", "prompt.md", strings.TrimSpace(defaultPrompt))
}
