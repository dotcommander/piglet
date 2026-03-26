package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// providerAliases maps shorthand names to canonical provider names.
var providerAliases = map[string]string{
	"gemini":  "google",
	"vertex":  "google-vertex",
	"bedrock": "amazon-bedrock",
	"copilot": "github-copilot",
	"azure":   "azure-openai",
}

// NormalizeProvider resolves aliases to canonical provider names.
func NormalizeProvider(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return ""
	}
	if canonical, ok := providerAliases[n]; ok {
		return canonical
	}
	return n
}

// Auth manages API key resolution.
// Priority: runtime overrides → stored credentials → environment variables → command values.
type Auth struct {
	mu          sync.RWMutex
	path        string // empty = in-memory only
	credentials map[string]string
	runtime     map[string]string
}

// NewAuth creates an Auth that persists to the given path.
// Pass "" for in-memory only (tests).
func NewAuth(path string) *Auth {
	a := &Auth{
		path:        path,
		credentials: make(map[string]string),
		runtime:     make(map[string]string),
	}
	if path != "" {
		_ = a.load() // best effort
	}
	return a
}

// NewAuthDefault creates an Auth using the default auth path.
func NewAuthDefault() (*Auth, error) {
	path, err := AuthPath()
	if err != nil {
		return nil, err
	}
	return NewAuth(path), nil
}

// GetAPIKey returns the API key for a provider.
// Resolution order: runtime override → stored credential → env var → empty.
func (a *Auth) GetAPIKey(provider string) string {
	normalized := NormalizeProvider(provider)
	if normalized == "" {
		return ""
	}

	a.mu.RLock()
	rt := a.runtime[normalized]
	stored := a.credentials[normalized]
	a.mu.RUnlock()

	// Runtime override
	if rt != "" {
		return rt
	}

	// Stored credential (may be a literal, env ref, or command)
	if stored != "" {
		return resolveValue(stored)
	}

	// Environment variable: <PROVIDER>_API_KEY
	for _, envKey := range envKeyCandidates(normalized) {
		if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
			return v
		}
	}

	return ""
}

// SetRuntimeKey sets a runtime API key override (not persisted).
func (a *Auth) SetRuntimeKey(provider, key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.runtime[NormalizeProvider(provider)] = key
}

// SetKey stores an API key and persists to disk.
func (a *Auth) SetKey(provider, key string) error {
	normalized := NormalizeProvider(provider)
	a.mu.Lock()
	a.credentials[normalized] = key
	a.mu.Unlock()
	return a.save()
}

// RemoveKey removes a stored API key and persists.
func (a *Auth) RemoveKey(provider string) error {
	normalized := NormalizeProvider(provider)
	a.mu.Lock()
	delete(a.credentials, normalized)
	a.mu.Unlock()
	return a.save()
}

// HasAuth reports whether any form of auth exists for the provider.
func (a *Auth) HasAuth(provider string) bool {
	return a.GetAPIKey(provider) != ""
}

// Providers returns a list of providers with stored credentials.
func (a *Auth) Providers() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(a.credentials))
	for k := range a.credentials {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func (a *Auth) load() error {
	if a.path == "" {
		return nil
	}
	data, err := os.ReadFile(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read auth: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return json.Unmarshal(data, &a.credentials)
}

func (a *Auth) save() error {
	if a.path == "" {
		return nil
	}
	a.mu.Lock()
	data, err := json.MarshalIndent(a.credentials, "", "  ")
	a.mu.Unlock()
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(a.path), 0700); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}
	return AtomicWrite(a.path, data, 0600)
}

// envKeyCandidates returns possible environment variable names for a provider.
// e.g. "openai" → ["OPENAI_API_KEY"], "google-vertex" → ["GOOGLE_VERTEX_API_KEY", "GOOGLE-VERTEX_API_KEY"]
func envKeyCandidates(provider string) []string {
	upper := strings.ToUpper(provider)
	primary := strings.ReplaceAll(upper, "-", "_") + "_API_KEY"
	keys := []string{primary}

	raw := upper + "_API_KEY"
	if raw != primary {
		keys = append(keys, raw)
	}

	// Common aliases: GEMINI_API_KEY for google provider
	if provider == "google" {
		keys = append(keys, "GEMINI_API_KEY")
	}

	return keys
}

// resolveValue resolves a configured value that may be:
//   - "!command" → execute shell command, use stdout
//   - "$ENV_VAR" or "${ENV_VAR}" → resolve from environment
//   - literal string
func resolveValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	// Shell command
	if strings.HasPrefix(trimmed, "!") {
		cmd := strings.TrimSpace(trimmed[1:])
		if cmd == "" {
			return ""
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.CommandContext(ctx, "cmd", "/C", cmd)
		} else {
			c = exec.CommandContext(ctx, "sh", "-c", cmd)
		}
		out, err := c.Output()
		if err != nil {
			slog.Warn("credential command failed", "cmd", cmd, "error", err)
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	// Environment variable references
	if strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}") {
		return os.Getenv(trimmed[2 : len(trimmed)-1])
	}
	if strings.HasPrefix(trimmed, "$") {
		return os.Getenv(trimmed[1:])
	}

	return value
}
