package route

import (
	_ "embed"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

//go:embed defaults/config.yaml
var defaultConfig string

//go:embed defaults/intents.yaml
var defaultIntents string

//go:embed defaults/domains.yaml
var defaultDomains string

// Config holds route extension settings.
type Config struct {
	Weights             Weights    `yaml:"weights"`
	PrimaryThreshold    float64    `yaml:"primary_threshold"`
	MaxPrimary          int        `yaml:"max_primary"`
	MaxSecondary        int        `yaml:"max_secondary"`
	MessageHook         HookConfig `yaml:"message_hook"`
	TriggerKeywordRatio float64    `yaml:"trigger_keyword_ratio"`
}

// Weights holds scoring signal weights.
type Weights struct {
	Intent  float64 `yaml:"intent"`
	Domain  float64 `yaml:"domain"`
	Trigger float64 `yaml:"trigger"`
	Anti    float64 `yaml:"anti"`
}

// HookConfig holds message hook settings.
type HookConfig struct {
	Enabled       bool    `yaml:"enabled"`
	MinConfidence float64 `yaml:"min_confidence"`
}

// IntentsConfig holds the full intent taxonomy.
type IntentsConfig struct {
	Intents map[string]IntentDef `yaml:"intents"`
}

// IntentDef defines verbs and keywords for a single intent.
type IntentDef struct {
	Verbs    []string `yaml:"verbs"`
	Keywords []string `yaml:"keywords"`
}

// DomainsConfig holds the full domain taxonomy.
type DomainsConfig struct {
	Domains map[string]DomainDef `yaml:"domains"`
}

// DomainDef defines keywords, project markers, and file extensions for a domain.
type DomainDef struct {
	Keywords   []string `yaml:"keywords"`
	Projects   []string `yaml:"projects"`
	Extensions []string `yaml:"extensions"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Weights: Weights{
			Intent:  0.40,
			Domain:  0.30,
			Trigger: 0.30,
			Anti:    0.50,
		},
		PrimaryThreshold:    0.25,
		MaxPrimary:          5,
		MaxSecondary:        10,
		TriggerKeywordRatio: 0.7,
		MessageHook: HookConfig{
			Enabled:       true,
			MinConfidence: 0.10,
		},
	}
}

// LoadConfig loads config from ~/.config/piglet/extensions/route/.
func LoadConfig() Config {
	return xdg.LoadYAMLExt("route", "config.yaml", DefaultConfig())
}

// LoadIntents loads the intent taxonomy.
func LoadIntents() IntentsConfig {
	return xdg.LoadYAMLExt("route", "intents.yaml", IntentsConfig{})
}

// LoadDomains loads the domain taxonomy.
func LoadDomains() DomainsConfig {
	return xdg.LoadYAMLExt("route", "domains.yaml", DomainsConfig{})
}
