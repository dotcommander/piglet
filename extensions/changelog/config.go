package changelog

import (
	"slices"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

type TypeConfig struct {
	Label string `yaml:"label"`
	Emoji string `yaml:"emoji"`
	Order int    `yaml:"order"`
}

type Config struct {
	Types         map[string]TypeConfig `yaml:"types"`
	FallbackCount int                   `yaml:"fallback_count"`
}

func defaultConfig() Config {
	return Config{FallbackCount: 20}
}

func loadConfig() Config {
	return xdg.LoadYAMLExt("changelog", "changelog.yaml", defaultConfig())
}

func typeOrder(types map[string]TypeConfig) []string {
	keys := make([]string, 0, len(types))
	for k := range types {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		return types[a].Order - types[b].Order
	})
	return keys
}
