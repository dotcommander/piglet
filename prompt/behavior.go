package prompt

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

const behaviorOrder = 10 // earliest — before self-knowledge (20), project docs (30), git (40), memory (50)

// RegisterBehavior loads behavioral guidelines from ~/.config/piglet/behavior.md
// and registers them as the earliest prompt section. If the file doesn't exist,
// the section is skipped — run /config --setup to create defaults.
func RegisterBehavior(app *ext.App) {
	content := loadBehavior()
	if content == "" {
		return
	}

	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Guidelines",
		Content: content,
		Order:   behaviorOrder,
	})
}

func loadBehavior() string {
	dir, err := config.ConfigDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "behavior.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
