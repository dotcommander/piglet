package provider

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/core"
)

// keyCache stores resolved key_command results for the process lifetime.
// Key is "provider:command" string. Prevents re-executing expensive commands
// (e.g. `op read`, `security find-generic-password`) on every stream request.
var keyCache sync.Map

// envVarPattern matches strings that look like environment variable names:
// all uppercase letters, digits, and underscores, starting with a letter or underscore.
var envVarPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]+$`)

// providerConfig is loaded from ~/.config/piglet/extensions/provider/provider.yaml.
// It supports three pi-mono-inspired capabilities:
//   - Provider overrides: redirect existing providers through a proxy (baseUrl, headers)
//   - Custom providers: define new provider names mapping to existing wire formats
//   - Compat overrides: per-model quirk handling for OpenAI-compatible endpoints
type providerConfig struct {
	// Overrides apply baseUrl/headers to existing providers (openai, anthropic, google).
	// Key is the provider name.
	Overrides map[string]providerOverride `yaml:"overrides"`

	// Custom defines new provider names that map to an existing wire format.
	// Key is the new provider name (e.g. "azure", "openrouter").
	Custom map[string]customProvider `yaml:"custom"`

	// Compat defines per-model quirk overrides. Keys are model ID prefixes.
	// The longest matching prefix wins.
	Compat map[string]compatOverride `yaml:"compat"`
}

// providerOverride lets users redirect an existing provider through a proxy.
//
// Example YAML:
//
//	overrides:
//	  anthropic:
//	    base_url: https://gateway.corp.com/anthropic
//	    headers:
//	      X-Corp-Auth: "token123"
//	    key_command: "security find-generic-password -ws anthropic-api"
type providerOverride struct {
	BaseURL    string            `yaml:"base_url"`
	Headers    map[string]string `yaml:"headers"`
	KeyCommand string            `yaml:"key_command"`
}

// customProvider defines a new provider that maps to an existing wire format.
//
// Example YAML:
//
//	custom:
//	  openrouter:
//	    api: openai
//	    base_url: https://openrouter.ai/api/v1
//	    headers:
//	      HTTP-Referer: "https://piglet.dev"
//	    key_command: "op read 'op://Dev/OpenRouter/api-key'"
type customProvider struct {
	API        string            `yaml:"api"`      // "openai", "anthropic", "google"
	BaseURL    string            `yaml:"base_url"` // required
	Headers    map[string]string `yaml:"headers"`
	KeyCommand string            `yaml:"key_command"`
}

// compatOverride handles quirks for OpenAI-compatible endpoints that deviate
// from the standard. Keys in the compat map are either exact model IDs or
// prefix patterns (trailing dash convention). Exact ID match takes priority,
// then longest matching prefix wins.
//
// Example YAML:
//
//	compat:
//	  deepseek-:
//	    strip_tool_choice: true
//	    headers:
//	      X-Custom: "value"
//	  deepseek-coder-v2:
//	    name: "DeepSeek Coder v2 (Custom)"
//	  mistral-:
//	    strip_stream_options: true
type compatOverride struct {
	Name               string            `yaml:"name"`
	StripToolChoice    bool              `yaml:"strip_tool_choice"`
	StripStreamOptions bool              `yaml:"strip_stream_options"`
	Headers            map[string]string `yaml:"headers"`
}

func defaultProviderConfig() providerConfig {
	return providerConfig{}
}

func loadProviderConfig() providerConfig {
	return xdg.LoadYAMLExt("provider", "provider.yaml", defaultProviderConfig())
}

// applyOverrides modifies the model in-place based on provider overrides
// and compat config. Returns the (possibly modified) model.
func applyOverrides(cfg providerConfig, model *core.Model) {
	// 1. Provider-level override (proxy)
	if ov, ok := cfg.Overrides[model.Provider]; ok {
		if ov.BaseURL != "" {
			model.BaseURL = ov.BaseURL
		}
		model.Headers = mergeHeaders(model.Headers, ov.Headers)
	}

	// 2. Custom provider — if the provider name matches a custom definition,
	// apply its config. The API field on the model may need to be set so the
	// stream handler constructs the right provider implementation.
	if cp, ok := cfg.Custom[model.Provider]; ok {
		if cp.BaseURL != "" && model.BaseURL == "" {
			model.BaseURL = cp.BaseURL
		}
		if cp.API != "" && model.API == "" {
			model.API = core.API(cp.API)
		}
		model.Headers = mergeHeaders(model.Headers, cp.Headers)
	}

	// 3. Compat overrides — exact ID first, then longest prefix match
	if co, found := matchCompat(cfg.Compat, model.ID); found {
		model.Headers = mergeHeaders(model.Headers, co.Headers)
		if co.Name != "" {
			model.Name = co.Name
		}
	}
}

// matchCompat finds the best compat override for a model ID.
// Exact ID match takes priority, then longest prefix match wins.
func matchCompat(compat map[string]compatOverride, modelID string) (compatOverride, bool) {
	// Exact match first
	if co, ok := compat[modelID]; ok {
		return co, true
	}

	// Longest prefix match
	var best compatOverride
	bestLen := 0
	for prefix, co := range compat {
		if prefix == modelID {
			continue // already checked
		}
		if strings.HasPrefix(modelID, prefix) && len(prefix) > bestLen {
			best = co
			bestLen = len(prefix)
		}
	}
	return best, bestLen > 0
}

// resolveKeyCommand returns the key_command for a provider from the config.
// Checks overrides first, then custom providers.
func resolveKeyCommand(cfg providerConfig, provider string) string {
	if ov, ok := cfg.Overrides[provider]; ok && ov.KeyCommand != "" {
		return ov.KeyCommand
	}
	if cp, ok := cfg.Custom[provider]; ok && cp.KeyCommand != "" {
		return cp.KeyCommand
	}
	return ""
}

// resolveKey resolves a key_command value using three formats:
//   - Starts with "!" → shell command (e.g. "!security find-generic-password -ws api")
//   - Matches [A-Z_][A-Z0-9_]+ → env var name (e.g. "MY_API_KEY")
//   - Otherwise → literal value
//
// Shell command results are cached for the process lifetime to avoid
// re-executing expensive credential lookups (1Password, macOS Keychain)
// on every stream request.
func resolveKey(ctx context.Context, provider, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}

	// Shell command: "!command args..."
	if strings.HasPrefix(value, "!") {
		return execKeyCommandCached(ctx, provider, value[1:])
	}

	// Env var indirection: "MY_CUSTOM_KEY" → os.Getenv("MY_CUSTOM_KEY")
	if envVarPattern.MatchString(value) {
		return os.Getenv(value), nil
	}

	// Literal value
	return value, nil
}

// execKeyCommandCached runs a shell command with process-lifetime caching.
func execKeyCommandCached(ctx context.Context, provider, command string) (string, error) {
	cacheKey := provider + ":" + command

	if cached, ok := keyCache.Load(cacheKey); ok {
		return cached.(string), nil
	}

	result, err := execKeyCommand(ctx, command)
	if err != nil {
		return "", err
	}

	keyCache.Store(cacheKey, result)
	return result, nil
}

// execKeyCommand runs a shell command to retrieve an API key.
// Returns the trimmed stdout output or an error. Times out after 5 seconds.
func execKeyCommand(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// mergeHeaders merges extra headers into base, returning the combined map.
// Extra headers take precedence over base on conflict.
func mergeHeaders(base, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(extra))
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}
