package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// Settings holds user configuration.
type Settings struct {
	DefaultProvider        string            `yaml:"defaultProvider,omitempty"`
	DefaultModel           string            `yaml:"defaultModel,omitempty"`
	SmallModel             string            `yaml:"smallModel,omitempty"`
	SystemPrompt           string            `yaml:"systemPrompt,omitempty"` // base identity; overridden by prompt.md
	Theme                  string            `yaml:"theme,omitempty"`
	Extensions             []string          `yaml:"extensions,omitempty"`
	Providers              map[string]string `yaml:"providers,omitempty"` // provider name → base URL override
	Agent                  AgentSettings     `yaml:"agent,omitempty"`
	Git                    GitSettings       `yaml:"git,omitempty"`
	Tools                  ToolSettings      `yaml:"tools,omitempty"`
	Bash                   BashSettings      `yaml:"bash,omitempty"`
	Shortcuts              map[string]string `yaml:"shortcuts,omitempty"`              // action → keybind (e.g. "model": "ctrl+p")
	PromptOrder            map[string]int    `yaml:"promptOrder,omitempty"`            // section title → order override
	RTK                    *bool             `yaml:"rtk,omitempty"`                    // nil = auto-detect; true/false = explicit
	Debug                  bool              `yaml:"debug,omitempty"`                  // log all request/response payloads
	Safeguard              *bool             `yaml:"safeguard,omitempty"`              // nil/true = enabled; false = disabled
	DisabledExtensions     []string          `yaml:"disabled_extensions,omitempty"`    // extensions to skip during loading
	AllowProjectExtensions *bool             `yaml:"allowProjectExtensions,omitempty"` // default false; must opt-in for security
	SubAgent               SubAgentSettings  `yaml:"subagent,omitempty"`
	ExtInstall             ExtensionSettings `yaml:"extInstall,omitempty"`
	LocalServers           []string          `yaml:"localServers,omitempty"` // URLs of local model servers to probe on startup
	LocalDefaults          LocalDefaults     `yaml:"localDefaults,omitempty"`
	DeferredToolsNote      string            `yaml:"deferredToolsNote,omitempty"` // instruction shown when deferred tools are present
}

// DefaultMaxTurns is the application-wide default for agent max turns.
// Used as the fallback in config.IntOr(settings.Agent.MaxTurns, config.DefaultMaxTurns).
const DefaultMaxTurns = 30

// DefaultSubAgentMaxTurns is the default max turns for sub-agent dispatch
// (SubAgentSettings.MaxTurns zero value).
const DefaultSubAgentMaxTurns = 30

// AgentSettings controls agent loop behavior. Zero values use defaults.
type AgentSettings struct {
	MaxTurns          int   `yaml:"maxTurns,omitempty"`          // default DefaultMaxTurns
	BgMaxTurns        int   `yaml:"bgMaxTurns,omitempty"`        // default 5
	AutoTitle         *bool `yaml:"autoTitle,omitempty"`         // default true; pointer distinguishes false from unset
	CompactKeepRecent int   `yaml:"compactKeepRecent,omitempty"` // default 6
	CompactAt         int   `yaml:"compactAt,omitempty"`         // token threshold for auto-compact; 0 = disabled
	MaxMessages       int   `yaml:"maxMessages,omitempty"`       // hard cap on messages; 0 = unlimited
	MaxTokens         int   `yaml:"maxTokens,omitempty"`         // output token limit; 0 = use model default
	MaxRetries        int   `yaml:"maxRetries,omitempty"`        // retry attempts on error; 0 = use default (3)
	ToolConcurrency   int   `yaml:"toolConcurrency,omitempty"`   // max parallel tool calls; 0 = use default (10)
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

// ToolSettings controls built-in tool defaults. Zero values use defaults.
type ToolSettings struct {
	ReadLimit int `yaml:"readLimit,omitempty"` // max lines per read; default 2000
	GrepLimit int `yaml:"grepLimit,omitempty"` // max grep matches; default 100
}

// BashSettings controls the bash tool limits. Zero values use defaults.
type BashSettings struct {
	DefaultTimeout int `yaml:"defaultTimeout,omitempty"` // seconds, default 30
	MaxTimeout     int `yaml:"maxTimeout,omitempty"`     // seconds, default 300
	MaxStdout      int `yaml:"maxStdout,omitempty"`      // bytes, default 100000
	MaxStderr      int `yaml:"maxStderr,omitempty"`      // bytes, default 50000
}

// SubAgentSettings controls the dispatch tool's sub-agent defaults.
type SubAgentSettings struct {
	MaxTurns int `yaml:"maxTurns,omitempty"` // default DefaultSubAgentMaxTurns
}

// ExtensionSettings controls extension installation defaults.
type ExtensionSettings struct {
	RepoURL  string   `yaml:"repoUrl,omitempty"`
	Official []string `yaml:"official,omitempty"`
}

// LocalDefaults holds default values for ad-hoc local model connections.
type LocalDefaults struct {
	ContextWindow int `yaml:"contextWindow,omitempty"`
	MaxTokens     int `yaml:"maxTokens,omitempty"`
}

// applyDefaults fills in zero-value fields with their defaults.
func applyDefaults(s *Settings) {
	if s.ExtInstall.RepoURL == "" || s.ExtInstall.RepoURL == "https://github.com/dotcommander/piglet-extensions.git" {
		s.ExtInstall.RepoURL = "https://github.com/dotcommander/piglet.git"
	}
	if len(s.ExtInstall.Official) == 0 {
		s.ExtInstall.Official = []string{
			"pack-core", "pack-agent", "pack-context", "pack-code", "pack-workflow", "pack-cron",
			"mcp",
		}
	}
}

// DefaultSettings returns a Settings with all defaults applied.
// Useful for testing and for code that needs a baseline configuration.
func DefaultSettings() Settings {
	var s Settings
	applyDefaults(&s)
	return s
}

// ConfigDir returns ~/.config/piglet/, respecting XDG_CONFIG_HOME.
func ConfigDir() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config dir: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "piglet"), nil
}

// configPath joins ConfigDir() with the given path elements.
// Reduces boilerplate across all path resolver functions.
func configPath(suffix ...string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{dir}, suffix...)...), nil
}

// ExtensionConfigDir returns the namespaced config directory for an extension:
// ~/.config/piglet/extensions/<name>/.
// It does not create the directory.
func ExtensionConfigDir(name string) (string, error) {
	return configPath("extensions", name)
}

// SessionsDir returns ~/.config/piglet/sessions/.
func SessionsDir() (string, error) {
	return configPath("sessions")
}

// AuthPath returns ~/.config/piglet/auth.json.
func AuthPath() (string, error) {
	return configPath("auth.json")
}

// SettingsPath returns ~/.config/piglet/config.yaml.
func SettingsPath() (string, error) {
	return configPath("config.yaml")
}

// HistoryPath returns ~/.config/piglet/history.
func HistoryPath() (string, error) {
	return configPath("history")
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
			var s Settings
			applyDefaults(&s)
			return s, nil
		}
		return Settings{}, fmt.Errorf("read config: %w", err)
	}

	var s Settings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("parse config: %w", err)
	}
	applyDefaults(&s)
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
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return AtomicWrite(path, data, 0600)
}

// AtomicWrite writes data to path using a temp file + rename pattern.
// Creates parent directories with 0o700 if needed.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	cleanup = false
	return os.Rename(tmpPath, path)
}

// IntOr returns v if positive, otherwise fallback.
func IntOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}

// ResolveSmallModel returns the first non-empty value from the small model
// cascade: PIGLET_SMALL_MODEL env → SmallModel config → PIGLET_DEFAULT_MODEL env → DefaultModel config.
func (s Settings) ResolveSmallModel() string {
	if v := os.Getenv("PIGLET_SMALL_MODEL"); v != "" {
		return v
	}
	if s.SmallModel != "" {
		return s.SmallModel
	}
	return s.ResolveDefaultModel()
}

// ResolveDefaultModel returns the first non-empty value from the default model
// cascade: PIGLET_DEFAULT_MODEL env → DefaultModel config.
func (s Settings) ResolveDefaultModel() string {
	if v := os.Getenv("PIGLET_DEFAULT_MODEL"); v != "" {
		return v
	}
	return s.DefaultModel
}

// IsExtensionDisabled reports whether the named extension is in the disabled list.
func (s Settings) IsExtensionDisabled(name string) bool {
	return slices.Contains(s.DisabledExtensions, name)
}
