// Package sop implements the /sop plan command: loads a structured planning
// SOP from a three-tier resolution chain and prepends it to the user's topic.
//
// Resolution order: project-local → global → embedded default.
// The embedded default is the last-resort tier; users override it by editing
// ~/.config/piglet/sops/plan.md (global) or <cwd>/.piglet/sops/plan.md (project).
package sop

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed default_plan.md
var defaultPlan string

// Resolve returns SOP content and the source label ("project", "global", or "embedded").
//
// Resolution order:
//  1. <cwd>/.piglet/sops/plan.md   — project-local override
//  2. ~/.config/piglet/sops/plan.md — user global override
//  3. embedded default_plan.md      — last resort
//
// cwd == "" skips the project-local tier.
// Errors other than fs.ErrNotExist (e.g. permission denied) surface immediately
// rather than silently falling back to a lower tier.
func Resolve(cwd string) (content, source string, err error) {
	// Tier 1: project-local.
	if cwd != "" {
		projectPath := filepath.Join(cwd, ".piglet", "sops", "plan.md")
		c, err := readSOPFile(projectPath)
		if err == nil {
			return c, "project", nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", fmt.Errorf("sop: read project SOP: %w", err)
		}
	}

	// Tier 2: global user config.
	globalPath, err := globalSOPPath()
	if err != nil {
		return "", "", fmt.Errorf("sop: resolve global path: %w", err)
	}
	c, err := readSOPFile(globalPath)
	if err == nil {
		return c, "global", nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", "", fmt.Errorf("sop: read global SOP: %w", err)
	}

	// Tier 3: embedded default.
	return defaultPlan, "embedded", nil
}

// EnsureGlobalDefault writes the embedded default to ~/.config/piglet/sops/plan.md
// if the file does not already exist. Idempotent — safe to call on every init.
func EnsureGlobalDefault() error {
	path, err := globalSOPPath()
	if err != nil {
		return fmt.Errorf("sop: resolve global path: %w", err)
	}

	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("sop: stat global SOP: %w", err)
	}

	if err := xdg.WriteFileAtomic(path, []byte(defaultPlan)); err != nil {
		return fmt.Errorf("sop: write global default: %w", err)
	}
	return nil
}

// globalSOPPath returns ~/.config/piglet/sops/plan.md.
func globalSOPPath() (string, error) {
	cfgDir, err := xdg.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "sops", "plan.md"), nil
}

// readSOPFile reads a SOP file and returns its content.
// Returns fs.ErrNotExist if the file is missing; other errors surface unchanged.
func readSOPFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
