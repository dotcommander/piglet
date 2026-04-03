package suggest

import (
	_ "embed"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed defaults/prompt.md
var defaultPrompt string

// Config holds suggestion settings.
type Config struct {
	Model       string        `yaml:"model"`
	MaxTokens   int           `yaml:"max_tokens"`
	Timeout     time.Duration `yaml:"timeout"`
	Cooldown    int           `yaml:"cooldown"`
	Enabled     bool          `yaml:"enabled"`
	TriggerMode string        `yaml:"trigger_mode"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Model:       "small",
		MaxTokens:   50,
		Timeout:     5 * time.Second,
		Cooldown:    3,
		Enabled:     true,
		TriggerMode: "auto",
	}
}

// LoadConfig loads config from ~/.config/piglet/suggest.yaml, creating defaults if missing.
func LoadConfig() Config {
	return xdg.LoadYAMLExt("suggest", "suggest.yaml", DefaultConfig())
}

// LoadPrompt loads the prompt template from the extension's namespaced directory,
// creating the default if missing.
func LoadPrompt() string {
	return xdg.LoadOrCreateExt("suggest", "prompt.md", DefaultPrompt())
}

// DefaultPrompt returns the default suggestion prompt template.
func DefaultPrompt() string {
	return strings.TrimSpace(defaultPrompt)
}
