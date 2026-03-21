package subagent

import "github.com/dotcommander/piglet/ext"

// Config holds sub-agent defaults from config.yaml.
type Config struct {
	MaxTurns int // default 10
}

// Register sets up the dispatch tool for sub-agent delegation.
func Register(app *ext.App, cfg Config) {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 10
	}
	// Cache prompt at startup to avoid disk reads per dispatch
	prompt := loadPrompt()
	registerTool(app, cfg, prompt)
}
