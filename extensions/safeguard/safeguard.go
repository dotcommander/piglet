// Package safeguard blocks dangerous commands before execution.
// Configuration loaded from ~/.config/piglet/safeguard.yaml.
// Supports three profiles: strict (block + workspace scoping), balanced (block only), off (log only).
package safeguard

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

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

// BlockerWithConfig returns a Before interceptor that enforces the given profile.
// In strict mode, write/edit/bash calls targeting paths outside cwd are blocked.
// Returns (allow, args, reason) — reason is non-empty when allow is false.
// Blocked decisions are logged to the audit logger when provided.
func BlockerWithConfig(cfg Config, compiled []*regexp.Regexp, cwd string, audit *AuditLogger) func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, string) {
	return func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, string) {
		// Strict mode: workspace scoping for file-mutating tools
		if cfg.Profile == ProfileStrict && cwd != "" {
			switch toolName {
			case "write", "edit", "multi_edit":
				path, _ := args["file_path"].(string)
				if path != "" && !isInsideWorkspace(path, cwd) {
					audit.Log(toolName, "blocked", "outside workspace", path)
					return false, nil, fmt.Sprintf("safeguard [strict]: blocked %s outside workspace %s", toolName, cwd)
				}
			}
		}

		// Pattern matching applies to bash in both strict and balanced
		if toolName == "bash" {
			command, _ := args["command"].(string)
			if command != "" {
				// Fast path: skip security checks for known read-only commands.
				if ClassifyCommand(command) == CommandReadOnly {
					audit.Log(toolName, "allowed", "read_only", truncate(command, 200))
					return true, args, ""
				}

				// Metacharacter injection checks (parser-level attacks).
				if err := ValidateInjection(command); err != nil {
					audit.Log(toolName, "blocked", err.Error(), truncate(command, 200))
					return false, nil, fmt.Sprintf("safeguard: %v", err)
				}

				for _, re := range compiled {
					if re.MatchString(command) {
						audit.Log(toolName, "blocked", re.String(), truncate(command, 200))
						configPath := "safeguard.yaml"
						if dir, err := xdg.ExtensionDir("safeguard"); err == nil {
							configPath = filepath.Join(dir, "safeguard.yaml")
						}
						return false, nil, fmt.Sprintf("safeguard: blocked dangerous command matching %q — edit %s to adjust", re.String(), configPath)
					}
				}
			}
		}

		return true, args, ""
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
