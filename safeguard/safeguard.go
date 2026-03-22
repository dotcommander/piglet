// Package safeguard blocks dangerous bash commands before execution.
// Patterns are loaded from ~/.config/piglet/safeguard.yaml.
// If the file doesn't exist and safeguard is enabled, a default is created.
package safeguard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
	"gopkg.in/yaml.v3"
)

// Register adds the safeguard interceptor to app.
// enabled: nil/true = enabled; false = skip.
func Register(app *ext.App, enabled *bool) {
	if enabled != nil && !*enabled {
		return
	}

	compiled := CompilePatterns(LoadPatterns())
	if len(compiled) == 0 {
		return
	}

	app.RegisterInterceptor(ext.Interceptor{
		Name:     "safeguard",
		Priority: 2000, // security — runs before everything else
		Before:   Blocker(compiled),
	})
}

// CompilePatterns compiles string patterns into case-insensitive regexps.
// Invalid patterns are silently skipped.
func CompilePatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			continue
		}
		compiled = append(compiled, re)
	}
	return compiled
}

// Blocker returns a Before interceptor function that checks commands against patterns.
func Blocker(patterns []*regexp.Regexp) func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
	return func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
		if toolName != "bash" {
			return true, args, nil
		}

		command, _ := args["command"].(string)
		if command == "" {
			return true, args, nil
		}

		for _, re := range patterns {
			if re.MatchString(command) {
				return false, nil, fmt.Errorf("safeguard: blocked dangerous command matching %q — edit ~/.config/piglet/safeguard.yaml to adjust", re.String())
			}
		}

		return true, args, nil
	}
}

type safeguardFile struct {
	Patterns []string `yaml:"patterns"`
}

// LoadPatterns reads patterns from ~/.config/piglet/safeguard.yaml.
// Creates a default file if it doesn't exist.
func LoadPatterns() []string {
	dir, err := config.ConfigDir()
	if err != nil {
		return nil
	}

	path := filepath.Join(dir, "safeguard.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return createDefault(path)
		}
		return nil
	}

	var f safeguardFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil
	}
	return f.Patterns
}

func createDefault(path string) []string {
	dir := filepath.Dir(path)

	// Read the seed from safeguard-default.yaml next to config
	seedPath := filepath.Join(dir, "safeguard-default.yaml")
	if _, err := os.Stat(seedPath); err == nil {
		// Seed exists — copy it
		data, err := os.ReadFile(seedPath)
		if err == nil {
			_ = os.WriteFile(path, data, 0600)
			var f safeguardFile
			if yaml.Unmarshal(data, &f) == nil {
				return f.Patterns
			}
		}
	}

	// No seed file — build default patterns and write them
	patterns := defaultPatterns()
	f := safeguardFile{Patterns: patterns}
	data, err := yaml.Marshal(f)
	if err != nil {
		return patterns
	}

	var b strings.Builder
	b.WriteString("# Safeguard: dangerous command patterns (regex, case-insensitive)\n")
	b.WriteString("# Edit this file to add/remove patterns. Set safeguard: false in config.yaml to disable.\n\n")
	b.Write(data)

	_ = os.MkdirAll(filepath.Dir(path), 0700)
	_ = os.WriteFile(path, []byte(b.String()), 0600)

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
