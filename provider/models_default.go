package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CuratedModel defines a model in the default catalog.
// This is the single source of truth for the model list — used by both
// the embedded fallback YAML and the modelsdev YAML generator.
type CuratedModel struct {
	ID            string
	Name          string
	Provider      string
	API           string // "openai", "anthropic", "google"
	BaseURL       string
	ContextWindow int // default when API data unavailable
	MaxTokens     int // default when API data unavailable
}

// CuratedModels returns the default model catalog.
func CuratedModels() []CuratedModel { return curatedModels }

var curatedModels = []CuratedModel{
	// Anthropic
	{ID: "claude-opus-4-6", Name: "Claude Opus 4.6", Provider: "anthropic", API: "anthropic", BaseURL: "https://api.anthropic.com", ContextWindow: 1000000, MaxTokens: 128000},
	{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Provider: "anthropic", API: "anthropic", BaseURL: "https://api.anthropic.com", ContextWindow: 1000000, MaxTokens: 64000},
	{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", API: "anthropic", BaseURL: "https://api.anthropic.com", ContextWindow: 200000, MaxTokens: 64000},
	{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Provider: "anthropic", API: "anthropic", BaseURL: "https://api.anthropic.com", ContextWindow: 200000, MaxTokens: 64000},
	// OpenAI
	{ID: "gpt-5.4", Name: "GPT-5.4", Provider: "openai", API: "openai", BaseURL: "https://api.openai.com", ContextWindow: 1050000, MaxTokens: 128000},
	{ID: "gpt-5", Name: "GPT-5", Provider: "openai", API: "openai", BaseURL: "https://api.openai.com", ContextWindow: 400000, MaxTokens: 128000},
	{ID: "o4-mini", Name: "o4-mini", Provider: "openai", API: "openai", BaseURL: "https://api.openai.com", ContextWindow: 200000, MaxTokens: 100000},
	{ID: "gpt-4.1", Name: "GPT-4.1", Provider: "openai", API: "openai", BaseURL: "https://api.openai.com", ContextWindow: 1047576, MaxTokens: 32768},
	{ID: "gpt-4.1-mini", Name: "GPT-4.1 mini", Provider: "openai", API: "openai", BaseURL: "https://api.openai.com", ContextWindow: 1047576, MaxTokens: 32768},
	{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", API: "openai", BaseURL: "https://api.openai.com", ContextWindow: 128000, MaxTokens: 16384},
	{ID: "o3", Name: "o3", Provider: "openai", API: "openai", BaseURL: "https://api.openai.com", ContextWindow: 200000, MaxTokens: 100000},
	// Google
	{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro Preview", Provider: "google", API: "google", BaseURL: "https://generativelanguage.googleapis.com", ContextWindow: 1048576, MaxTokens: 65536},
	{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Provider: "google", API: "google", BaseURL: "https://generativelanguage.googleapis.com", ContextWindow: 1048576, MaxTokens: 65536},
	{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Provider: "google", API: "google", BaseURL: "https://generativelanguage.googleapis.com", ContextWindow: 1048576, MaxTokens: 65536},
	// xAI
	{ID: "grok-3", Name: "Grok 3", Provider: "xai", API: "openai", BaseURL: "https://api.x.ai", ContextWindow: 131072, MaxTokens: 8192},
	// Groq (free/fast inference)
	{ID: "llama-3.3-70b-versatile", Name: "Llama 3.3 70B", Provider: "groq", API: "openai", BaseURL: "https://api.groq.com/openai", ContextWindow: 131072, MaxTokens: 32768},
	// OpenRouter
	{ID: "auto", Name: "Auto (best available)", Provider: "openrouter", API: "openai", BaseURL: "https://openrouter.ai/api", ContextWindow: 200000, MaxTokens: 16384},
	// Z.AI (GLM models)
	{ID: "glm-5", Name: "GLM-5", Provider: "zai", API: "openai", BaseURL: "https://api.z.ai/api/coding/paas/v4", ContextWindow: 128000, MaxTokens: 8192},
	{ID: "glm-4.7", Name: "GLM-4.7", Provider: "zai", API: "openai", BaseURL: "https://api.z.ai/api/coding/paas/v4", ContextWindow: 128000, MaxTokens: 8192},
	{ID: "glm-5-turbo", Name: "GLM-5 Turbo", Provider: "zai", API: "openai", BaseURL: "https://api.z.ai/api/coding/paas/v4", ContextWindow: 128000, MaxTokens: 8192},
	// LM Studio (local)
	{ID: "local-model", Name: "Local Model", Provider: "lmstudio", API: "openai", BaseURL: "http://localhost:1234", ContextWindow: 32000, MaxTokens: 32000},
}

// DefaultModelsYAML returns the default models catalog as YAML.
// The result is cached since the curated list is immutable.
var defaultModelsOnce sync.Once
var defaultModelsCache string

func DefaultModelsYAML() string {
	defaultModelsOnce.Do(func() {
		defaultModelsCache = GenerateModelsYAML(curatedModels, nil)
	})
	return defaultModelsCache
}

// GenerateModelsYAML builds a models.yaml string from a curated model list.
// If overrides is non-nil, it maps "provider/id" to replacement ContextWindow
// and MaxTokens values (used by modelsdev to inject API data).
func GenerateModelsYAML(models []CuratedModel, overrides map[string]CuratedModelOverride) string {
	var b strings.Builder
	b.WriteString("models:\n")

	prevProvider := ""
	for _, cm := range models {
		if cm.Provider != prevProvider {
			fmt.Fprintf(&b, "  # %s\n", providerLabel(cm.Provider))
			prevProvider = cm.Provider
		}

		contextWindow := cm.ContextWindow
		maxTokens := cm.MaxTokens
		name := cm.Name

		if overrides != nil {
			if ov, ok := overrides[modelKey(cm.Provider, cm.ID)]; ok {
				if ov.ContextWindow > 0 {
					contextWindow = ov.ContextWindow
				}
				if ov.MaxTokens > 0 {
					maxTokens = ov.MaxTokens
				}
				if ov.Name != "" {
					name = ov.Name
				}
			}
		}

		fmt.Fprintf(&b, "  - id: %s\n", cm.ID)
		fmt.Fprintf(&b, "    name: %s\n", name)
		fmt.Fprintf(&b, "    provider: %s\n", cm.Provider)
		fmt.Fprintf(&b, "    api: %s\n", cm.API)
		fmt.Fprintf(&b, "    baseUrl: %s\n", cm.BaseURL)
		fmt.Fprintf(&b, "    contextWindow: %d\n", contextWindow)
		fmt.Fprintf(&b, "    maxTokens: %d\n", maxTokens)
		b.WriteString("\n")
	}

	return b.String()
}

// CuratedModelOverride holds API-sourced values that replace defaults.
type CuratedModelOverride struct {
	Name          string
	ContextWindow int
	MaxTokens     int
}

func providerLabel(provider string) string {
	switch provider {
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "google":
		return "Google"
	case "xai":
		return "xAI"
	case "groq":
		return "Groq (free/fast inference)"
	case "openrouter":
		return "OpenRouter"
	case "zai":
		return "Z.AI (GLM models)"
	case "lmstudio":
		return "LM Studio (local)"
	default:
		return provider
	}
}

// WriteDefaultModels writes the default models catalog to path.
func WriteDefaultModels(path string) error {
	return WriteModelsData(path, DefaultModelsYAML())
}

// WriteModelsData writes the given YAML content to path using an atomic write
// (temp file + rename). It creates the parent directory if needed.
// Returns nil without writing if the file already exists.
func WriteModelsData(path string, data string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return fmt.Errorf("write models data: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install models data: %w", err)
	}

	return nil
}
