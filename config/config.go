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
