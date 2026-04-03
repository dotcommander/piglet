package cron

import (
	"fmt"
	"path/filepath"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"gopkg.in/yaml.v3"
)

// Config is the top-level schedules.yaml structure.
type Config struct {
	Tasks map[string]TaskConfig `yaml:"tasks"`
}

// TaskConfig defines a single scheduled task.
type TaskConfig struct {
	Action   string            `yaml:"action"`  // "shell", "prompt", "webhook"
	Command  string            `yaml:"command"` // for action=shell
	Prompt   string            `yaml:"prompt"`  // for action=prompt
	URL      string            `yaml:"url"`     // for action=webhook
	Method   string            `yaml:"method"`  // HTTP method for webhook (default POST)
	Body     string            `yaml:"body"`    // HTTP body for webhook
	Headers  map[string]string `yaml:"headers"` // HTTP headers for webhook
	Schedule ScheduleSpec      `yaml:"schedule"`
	Timeout  string            `yaml:"timeout"`  // duration string, default "5m"
	Enabled  *bool             `yaml:"enabled"`  // nil = true (enabled by default)
	WorkDir  string            `yaml:"work_dir"` // for action=shell
	Env      map[string]string `yaml:"env"`      // extra env vars for shell
}

// IsEnabled returns whether the task is enabled (default true).
func (t TaskConfig) IsEnabled() bool {
	if t.Enabled == nil {
		return true
	}
	return *t.Enabled
}

// ScheduleSpec holds the schedule definition from YAML.
// Exactly one field should be set.
type ScheduleSpec struct {
	Every   string `yaml:"every"`    // duration string: "10m", "1h"
	DailyAt string `yaml:"daily_at"` // "HH:MM" in local time
	Weekly  string `yaml:"weekly"`   // "monday 09:00"
}

// DefaultConfig returns an empty config (no tasks).
func DefaultConfig() Config {
	return Config{Tasks: make(map[string]TaskConfig)}
}

// LoadConfig loads schedules.yaml from ~/.config/piglet/.
func LoadConfig() Config {
	return xdg.LoadYAMLExt("cron", "schedules.yaml", DefaultConfig())
}

// SaveConfig writes the config to ~/.config/piglet/extensions/cron/schedules.yaml atomically.
func SaveConfig(cfg Config) error {
	dir, err := xdg.ExtensionDir("cron")
	if err != nil {
		return fmt.Errorf("resolve cron dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return xdg.WriteFileAtomic(filepath.Join(dir, "schedules.yaml"), data)
}
