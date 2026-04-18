package safeguard

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"gopkg.in/yaml.v3"
)

const (
	ProfileStrict   = "strict"
	ProfileBalanced = "balanced"
	ProfileOff      = "off"
)

// Config holds safeguard configuration.
type Config struct {
	Profile  string   `yaml:"profile"`
	Patterns []string `yaml:"patterns"`
}

// LoadPatterns reads patterns from ~/.config/piglet/safeguard.yaml.
// Creates a default file if it doesn't exist.
// Retained for backward compatibility.
func LoadPatterns() []string {
	return LoadConfig().Patterns
}

// LoadConfig reads the full safeguard configuration.
// Tries the namespaced extension directory first, falls back to flat config dir.
func LoadConfig() Config {
	dir, err := xdg.ExtensionDir("safeguard")
	if err != nil {
		return Config{Profile: ProfileBalanced, Patterns: defaultPatterns()}
	}

	path := filepath.Join(dir, "safeguard.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return Config{Profile: ProfileBalanced, Patterns: defaultPatterns()}
		}
		// Fallback: try flat location
		flatDir, flatErr := xdg.ConfigDir()
		if flatErr != nil {
			return Config{Profile: ProfileBalanced, Patterns: defaultPatterns()}
		}
		flatPath := filepath.Join(flatDir, "safeguard.yaml")
		data, err = os.ReadFile(flatPath)
		if err != nil {
			if os.IsNotExist(err) {
				patterns := createDefault(path)
				return Config{Profile: ProfileBalanced, Patterns: patterns}
			}
			return Config{Profile: ProfileBalanced, Patterns: defaultPatterns()}
		}
		// Migrate from flat to namespaced
		_ = xdg.WriteFileAtomic(path, data)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{Profile: ProfileBalanced, Patterns: defaultPatterns()}
	}
	if cfg.Profile == "" {
		cfg.Profile = ProfileBalanced
	}
	return cfg
}

func createDefault(path string) []string {
	dir := filepath.Dir(path)

	// Read the seed from safeguard-default.yaml — check namespaced dir first, then flat
	seedPaths := []string{
		filepath.Join(dir, "safeguard-default.yaml"),
	}
	if flatDir, err := xdg.ConfigDir(); err == nil {
		seedPaths = append(seedPaths, filepath.Join(flatDir, "safeguard-default.yaml"))
	}
	for _, seedPath := range seedPaths {
		data, err := os.ReadFile(seedPath)
		if err == nil {
			_ = xdg.WriteFileAtomic(path, data)
			var cfg Config
			if yaml.Unmarshal(data, &cfg) == nil {
				return cfg.Patterns
			}
		}
	}

	// No seed file — build default config and write it
	patterns := defaultPatterns()
	cfg := Config{Profile: ProfileBalanced, Patterns: patterns}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return patterns
	}

	var b strings.Builder
	b.WriteString("# Safeguard configuration\n")
	b.WriteString("# Profile: strict (block + workspace scoping), balanced (block only), off (log only)\n")
	b.WriteString("# Edit patterns below. Set safeguard: false in config.yaml to disable entirely.\n\n")
	b.Write(data)

	_ = xdg.WriteFileAtomic(path, []byte(b.String()))

	return patterns
}

func defaultPatterns() []string {
	return []string{
		`\brm\s+-(r|f|rf|fr)\b`,
		`\brm\s+-\w*(r|f)\w*\s+/`,
		`\bsudo\s+rm\b`,
		`\bmkfs\b`,
		`\bdd\s+if=`,
		`\b(DROP|TRUNCATE)\s+(TABLE|DATABASE|SCHEMA)\b`,
		`\bDELETE\s+FROM\s+\S+\s*;?\s*$`,
		`\bgit\s+push\s+.*--force\b`,
		`\bgit\s+reset\s+--hard\b`,
		`\bgit\s+clean\s+-[dfx]`,
		`\bgit\s+branch\s+-D\b`,
		`\bchmod\s+-R\s+777\b`,
		`\bchown\s+-R\b`,
		`>\s*/dev/sd[a-z]`,
		`\b:()\s*\{\s*:\|:\s*&\s*\}\s*;?\s*:`,
		`\bkill\s+-9\s+-1\b`,
		`\bshutdown\b`,
		`\breboot\b`,
		`\bsystemctl\s+(stop|disable|mask)\b`,
	}
}
