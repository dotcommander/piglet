package tokengate

import "github.com/dotcommander/piglet/extensions/internal/xdg"

// RewriteRule defines a scope-limiting rewrite for a tool call.
type RewriteRule struct {
	Tool    string `yaml:"tool"`
	Pattern string `yaml:"pattern"`
	Action  string `yaml:"action"`
	Value   string `yaml:"value"`
}

// Config holds all tokengate settings: scope limiting + budget tracking.
type Config struct {
	// Scope limiter
	Enabled bool          `yaml:"enabled"`
	Rules   []RewriteRule `yaml:"rules"`

	// Budget tracking
	ContextWindow      int  `yaml:"context_window"`      // total context window tokens (default: 200000)
	WarnPercent        int  `yaml:"warn_percent"`         // warn at this % (default: 80)
	SummarizeThreshold int  `yaml:"summarize_threshold"`  // auto-summarize results larger than N chars (default: 8192)
	SummarizeEnabled   bool `yaml:"summarize_enabled"`    // enable auto-summarization (default: true)
}

// DefaultConfig returns sensible defaults for both scope limiting and budget tracking.
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		Rules: []RewriteRule{
			{Tool: "Bash", Pattern: `grep\s+-r\s+.*\.\*`, Action: "append_head", Value: "100"},
			{Tool: "Bash", Pattern: `find\s+/`, Action: "append_head", Value: "50"},
			{Tool: "Read", Pattern: "", Action: "limit_lines", Value: "200"},
		},
		ContextWindow:      200000,
		WarnPercent:        80,
		SummarizeThreshold: 8192,
		SummarizeEnabled:   true,
	}
}

// LoadConfig reads config from ~/.config/piglet/extensions/tokengate/.
func LoadConfig() Config {
	return xdg.LoadYAMLExt("tokengate", "config.yaml", DefaultConfig())
}
