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
	DefaultProvider    string            `yaml:"defaultProvider,omitempty"`
	DefaultModel       string            `yaml:"defaultModel,omitempty"`
	SmallModel         string            `yaml:"smallModel,omitempty"`
	SystemPrompt       string            `yaml:"systemPrompt,omitempty"` // base identity; overridden by prompt.md
	Theme              string            `yaml:"theme,omitempty"`
	ShellPath          string            `yaml:"shellPath,omitempty"`
	Extensions         []string          `yaml:"extensions,omitempty"`
	Providers          map[string]string `yaml:"providers,omitempty"` // provider name → base URL override
	Agent              AgentSettings     `yaml:"agent,omitempty"`
	Git                GitSettings       `yaml:"git,omitempty"`
	Tools              ToolSettings      `yaml:"tools,omitempty"`
	Bash               BashSettings      `yaml:"bash,omitempty"`
	Shortcuts          map[string]string `yaml:"shortcuts,omitempty"`           // action → keybind (e.g. "model": "ctrl+p")
	PromptOrder        map[string]int    `yaml:"promptOrder,omitempty"`         // section title → order override
	ProjectDocs        []ProjectDoc      `yaml:"projectDocs,omitempty"`         // files to auto-read for context
	RTK                *bool             `yaml:"rtk,omitempty"`                 // nil = auto-detect; true/false = explicit
	Debug              bool              `yaml:"debug,omitempty"`               // log all request/response payloads
	Safeguard          *bool             `yaml:"safeguard,omitempty"`           // nil/true = enabled; false = disabled
	DisabledExtensions []string          `yaml:"disabled_extensions,omitempty"` // extensions to skip during loading
	SubAgent           SubAgentSettings  `yaml:"subagent,omitempty"`
	ExtInstall         ExtensionSettings `yaml:"extInstall,omitempty"`
}

// ProjectDoc maps a filename to a prompt section title.
type ProjectDoc struct {
	Name  string `yaml:"name"`
	Title string `yaml:"title"`
}

// AgentSettings controls agent loop behavior. Zero values use defaults.
type AgentSettings struct {
	MaxTurns          int   `yaml:"maxTurns,omitempty"`          // default 10
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
	MaxTurns int `yaml:"maxTurns,omitempty"` // default 10
}

const defaultExtensionsRepoURL = "https://github.com/dotcommander/piglet-extensions.git"

var defaultOfficialExtensions = []string{
	"pack-core", "pack-agent", "pack-context", "pack-code", "pack-workflow",
	"mcp",
}

// ExtensionSettings controls extension installation defaults.
type ExtensionSettings struct {
	RepoURL  string   `yaml:"repoUrl,omitempty"`
	Official []string `yaml:"official,omitempty"`
}

// ResolveRepoURL returns the configured repo URL or the default.
func (s ExtensionSettings) ResolveRepoURL() string {
	if s.RepoURL != "" {
		return s.RepoURL
	}
	return defaultExtensionsRepoURL
}

// ResolveOfficial returns the configured extension list or the default.
func (s ExtensionSettings) ResolveOfficial() []string {
	if len(s.Official) > 0 {
		return s.Official
	}
	return defaultOfficialExtensions
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

// ExtensionConfigDir returns the namespaced config directory for an extension:
// ~/.config/piglet/extensions/<name>/.
// It does not create the directory.
func ExtensionConfigDir(name string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "extensions", name), nil
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

// HistoryPath returns ~/.config/piglet/history.
func HistoryPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history"), nil
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
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
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
