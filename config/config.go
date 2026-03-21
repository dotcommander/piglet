package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Settings holds user configuration.
type Settings struct {
	DefaultProvider string            `yaml:"defaultProvider,omitempty"`
	DefaultModel    string            `yaml:"defaultModel,omitempty"`
	SystemPrompt    string            `yaml:"systemPrompt,omitempty"` // base identity; overridden by prompt.md
	Theme           string            `yaml:"theme,omitempty"`
	ShellPath       string            `yaml:"shellPath,omitempty"`
	Extensions      []string          `yaml:"extensions,omitempty"`
	Providers       map[string]string `yaml:"providers,omitempty"` // provider name → base URL override
	Agent           AgentSettings     `yaml:"agent,omitempty"`
	Git             GitSettings       `yaml:"git,omitempty"`
	Bash            BashSettings      `yaml:"bash,omitempty"`
	Shortcuts       map[string]string `yaml:"shortcuts,omitempty"`  // action → keybind (e.g. "model": "ctrl+p")
	PromptOrder     map[string]int    `yaml:"promptOrder,omitempty"` // section title → order override
}

// AgentSettings controls agent loop behavior. Zero values use defaults.
type AgentSettings struct {
	MaxTurns        int   `yaml:"maxTurns,omitempty"`        // default 10
	BgMaxTurns      int   `yaml:"bgMaxTurns,omitempty"`      // default 5
	AutoTitle       *bool `yaml:"autoTitle,omitempty"`        // default true; pointer distinguishes false from unset
	CompactKeepRecent int `yaml:"compactKeepRecent,omitempty"` // default 6
}

// AutoTitleEnabled returns whether auto-title generation is on (default true).
func (a AgentSettings) AutoTitleEnabled() bool {
	if a.AutoTitle == nil {
		return true
	}
	return *a.AutoTitle
}

// GitSettings controls git context in the system prompt. Zero values use defaults.
type GitSettings struct {
	MaxDiffStatFiles int `yaml:"maxDiffStatFiles,omitempty"` // default 30
	MaxLogLines      int `yaml:"maxLogLines,omitempty"`      // default 5
	MaxDiffHunkLines int `yaml:"maxDiffHunkLines,omitempty"` // default 50
	CommandTimeout   int `yaml:"commandTimeout,omitempty"`   // seconds, default 5
}

// BashSettings controls the bash tool limits. Zero values use defaults.
type BashSettings struct {
	DefaultTimeout int `yaml:"defaultTimeout,omitempty"` // seconds, default 30
	MaxTimeout     int `yaml:"maxTimeout,omitempty"`     // seconds, default 300
	MaxStdout      int `yaml:"maxStdout,omitempty"`      // bytes, default 100000
	MaxStderr      int `yaml:"maxStderr,omitempty"`      // bytes, default 50000
}

// ConfigDir returns ~/.config/piglet/.
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(dir, "piglet"), nil
}

// SessionsDir returns ~/.config/piglet/sessions/.
func SessionsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// AuthPath returns ~/.config/piglet/auth.json.
func AuthPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

// SettingsPath returns ~/.config/piglet/config.yaml.
func SettingsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads settings from the config file. Returns zero Settings if file
// doesn't exist.
func Load() (Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		return Settings{}, err
	}
	return LoadFrom(path)
}

// LoadFrom reads settings from a specific path.
func LoadFrom(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("read config: %w", err)
	}

	var s Settings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("parse config: %w", err)
	}
	return s, nil
}

// Save writes settings to the config file.
func Save(s Settings) error {
	path, err := SettingsPath()
	if err != nil {
		return err
	}
	return SaveTo(s, path)
}

// SaveTo writes settings to a specific path.
func SaveTo(s Settings, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Atomic write: temp file then rename
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// Resolve returns the effective value for a setting, checking environment
// variables first (PIGLET_<KEY>), then the settings value, then the fallback.
func Resolve(envKey, settingsValue, fallback string) string {
	if v := strings.TrimSpace(os.Getenv("PIGLET_" + strings.ToUpper(envKey))); v != "" {
		return v
	}
	if strings.TrimSpace(settingsValue) != "" {
		return settingsValue
	}
	return fallback
}

// IntOr returns v if positive, otherwise fallback.
func IntOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
