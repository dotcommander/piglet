// Package safeguard blocks dangerous commands before execution.
// Configuration loaded from ~/.config/piglet/safeguard.yaml.
// Supports three profiles: strict (block + workspace scoping), balanced (block only), off (log only).
package safeguard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

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

	cfg := LoadConfig()
	if cfg.Profile == ProfileOff {
		return
	}

	compiled := CompilePatterns(cfg.Patterns)
	audit := NewAuditLogger()

	app.RegisterInterceptor(ext.Interceptor{
		Name:     "safeguard",
		Priority: 2000, // security — runs before everything else
		Before:   BlockerWithConfig(cfg, compiled, "", audit),
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
// Retained for backward compatibility with compiled-in registration.
func Blocker(patterns []*regexp.Regexp) func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
	return BlockerWithConfig(Config{Profile: ProfileBalanced}, patterns, "", nil)
}

// BlockerWithConfig returns a Before interceptor that enforces the given profile.
// In strict mode, write/edit/bash calls targeting paths outside cwd are blocked.
// Blocked decisions are logged to the audit logger when provided.
func BlockerWithConfig(cfg Config, compiled []*regexp.Regexp, cwd string, audit *AuditLogger) func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
	return func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
		// Strict mode: workspace scoping for file-mutating tools
		if cfg.Profile == ProfileStrict && cwd != "" {
			switch toolName {
			case "write", "edit", "multi_edit":
				path, _ := args["file_path"].(string)
				if path != "" && !isInsideWorkspace(path, cwd) {
					audit.Log(toolName, "blocked", "outside workspace", path)
					return false, nil, fmt.Errorf("safeguard [strict]: blocked %s outside workspace %s", toolName, cwd)
				}
			}
		}

		// Pattern matching applies to bash in both strict and balanced
		if toolName == "bash" {
			command, _ := args["command"].(string)
			if command != "" {
				for _, re := range compiled {
					if re.MatchString(command) {
						audit.Log(toolName, "blocked", re.String(), truncate(command, 200))
						return false, nil, fmt.Errorf("safeguard: blocked dangerous command matching %q — edit ~/.config/piglet/safeguard.yaml to adjust", re.String())
					}
				}
			}
		}

		return true, args, nil
	}
}

// isInsideWorkspace checks if path is under the workspace directory.
func isInsideWorkspace(path, cwd string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return false
	}
	// Ensure trailing separator for prefix check
	if !strings.HasSuffix(absCwd, string(filepath.Separator)) {
		absCwd += string(filepath.Separator)
	}
	return absPath == absCwd[:len(absCwd)-1] || strings.HasPrefix(absPath, absCwd)
}

// truncate returns the first n runes of s.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n]) + "..."
	}
	return s
}

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

// AuditLogger writes tool decisions to a JSONL file.
type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
}

type auditEntry struct {
	Timestamp string `json:"ts"`
	Tool      string `json:"tool"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// NewAuditLogger opens or creates the audit log file.
func NewAuditLogger() *AuditLogger {
	dir, err := config.ConfigDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(dir, "safeguard-audit.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil
	}
	return &AuditLogger{file: f}
}

// Log writes an audit entry. Safe for concurrent use.
func (a *AuditLogger) Log(tool, decision, reason, detail string) {
	if a == nil || a.file == nil {
		return
	}
	entry := auditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Tool:      tool,
		Decision:  decision,
		Reason:    reason,
		Detail:    detail,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	a.mu.Lock()
	_, _ = a.file.Write(data)
	a.mu.Unlock()
}

// Close closes the audit log file.
func (a *AuditLogger) Close() error {
	if a == nil || a.file == nil {
		return nil
	}
	return a.file.Close()
}

// LoadPatterns reads patterns from ~/.config/piglet/safeguard.yaml.
// Creates a default file if it doesn't exist.
// Retained for backward compatibility.
func LoadPatterns() []string {
	return LoadConfig().Patterns
}

// LoadConfig reads the full safeguard configuration.
func LoadConfig() Config {
	dir, err := config.ConfigDir()
	if err != nil {
		return Config{Profile: ProfileBalanced, Patterns: defaultPatterns()}
	}

	path := filepath.Join(dir, "safeguard.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			patterns := createDefault(path)
			return Config{Profile: ProfileBalanced, Patterns: patterns}
		}
		return Config{Profile: ProfileBalanced, Patterns: defaultPatterns()}
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

	// Read the seed from safeguard-default.yaml next to config
	seedPath := filepath.Join(dir, "safeguard-default.yaml")
	if _, err := os.Stat(seedPath); err == nil {
		// Seed exists — copy it
		data, err := os.ReadFile(seedPath)
		if err == nil {
			_ = os.WriteFile(path, data, 0600)
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
