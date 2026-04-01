package provider

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"gopkg.in/yaml.v3"
)

//go:embed defaults/models.yaml
var embeddedModelsYAML []byte

// CuratedModel defines a model in the default catalog.
// This is the single source of truth for the model list — used by both
// the embedded fallback YAML and the modelsdev YAML generator.
type CuratedModel struct {
	ID                  string         `yaml:"id"`
	Name                string         `yaml:"name"`
	Provider            string         `yaml:"provider"`
	API                 string         `yaml:"api"` // "openai", "anthropic", "google"
	BaseURL             string         `yaml:"baseUrl"`
	ContextWindow       int            `yaml:"contextWindow"`       // default when API data unavailable
	MaxTokens           int            `yaml:"maxTokens"`           // default when API data unavailable
	MaxCompletionTokens bool           `yaml:"maxCompletionTokens"` // true: use max_completion_tokens instead of max_tokens (OpenAI newer models)
	Cost                core.ModelCost `yaml:"cost"`
}

type curatedModelsFile struct {
	Models                      []CuratedModel    `yaml:"models"`
	MaxCompletionTokensPrefixes []string          `yaml:"maxCompletionTokensPrefixes"`
	ProviderLabels              map[string]string `yaml:"providerLabels"`
	SetupDefaultsMap            map[string]string `yaml:"setupDefaults"`
	LocalDefaults               struct {
		ContextWindow int `yaml:"contextWindow"`
		MaxTokens     int `yaml:"maxTokens"`
	} `yaml:"localDefaults"`
	DeferredToolsNote string `yaml:"deferredToolsNote"`
}

// parseModelsFile parses the embedded YAML on first call and caches the result.
var parseModelsFile = sync.OnceValue(func() curatedModelsFile {
	var f curatedModelsFile
	if err := yaml.Unmarshal(embeddedModelsYAML, &f); err != nil {
		panic(fmt.Sprintf("provider: failed to parse embedded models.yaml: %v", err))
	}
	return f
})

// CuratedModels returns the default model catalog.
func CuratedModels() []CuratedModel { return parseModelsFile().Models }

// SetupDefaults returns the per-provider default model IDs used during first-time setup.
func SetupDefaults() map[string]string { return parseModelsFile().SetupDefaultsMap }

// MaxCompletionTokensPrefixes returns model ID prefixes that require
// max_completion_tokens instead of max_tokens.
func MaxCompletionTokensPrefixes() []string { return parseModelsFile().MaxCompletionTokensPrefixes }

// LocalDefaultContextWindow returns the default context window for ad-hoc local models.
func LocalDefaultContextWindow() int { return parseModelsFile().LocalDefaults.ContextWindow }

// LocalDefaultMaxTokens returns the default max tokens for ad-hoc local models.
func LocalDefaultMaxTokens() int { return parseModelsFile().LocalDefaults.MaxTokens }

// DeferredToolsNote returns the instruction shown when deferred tools are present.
func DeferredToolsNote() string { return parseModelsFile().DeferredToolsNote }

// DefaultModelsYAML returns the default models catalog as YAML.
// The result is cached since the curated list is immutable.
var defaultModelsYAML = sync.OnceValue(func() string {
	return GenerateModelsYAML(parseModelsFile().Models, nil)
})

func DefaultModelsYAML() string {
	return defaultModelsYAML()
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
		if cm.Cost.Input > 0 || cm.Cost.Output > 0 {
			b.WriteString("    cost:\n")
			fmt.Fprintf(&b, "      input: %g\n", cm.Cost.Input)
			fmt.Fprintf(&b, "      output: %g\n", cm.Cost.Output)
			if cm.Cost.CacheRead > 0 {
				fmt.Fprintf(&b, "      cacheRead: %g\n", cm.Cost.CacheRead)
			}
			if cm.Cost.CacheWrite > 0 {
				fmt.Fprintf(&b, "      cacheWrite: %g\n", cm.Cost.CacheWrite)
			}
		}
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
	labels := parseModelsFile().ProviderLabels
	if label, ok := labels[provider]; ok {
		return label
	}
	// Fallback: title-case the key
	if len(provider) == 0 {
		return provider
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
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
	return config.AtomicWrite(path, []byte(data), 0o644)
}
