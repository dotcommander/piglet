package config

import (
	"fmt"
	"os"
)

// providerCandidate holds detected provider info from environment.
type providerCandidate struct {
	provider string
	envKey   string
	model    string
}

// ModelsPath returns the path to models.yaml in the config dir.
func ModelsPath() (string, error) {
	return configPath("models.yaml")
}

// NeedsSetup returns true if config.yaml or models.yaml is missing.
func NeedsSetup() bool {
	cfgPath, err := SettingsPath()
	if err != nil {
		return true
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return true
	}

	modPath, err := ModelsPath()
	if err != nil {
		return true
	}
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		return true
	}

	return false
}

// RunSetup runs the interactive first-time setup flow.
// writeModels is called to write the default models.yaml file.
// modelDefaults maps provider key → default model ID (used during key detection).
func RunSetup(writeModels func(path string) error, modelDefaults map[string]string) error {
	fmt.Println("piglet — first-time setup")
	fmt.Println()
	fmt.Println("Creating ~/.config/piglet/...")

	dir, err := ConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Write models.yaml
	modPath, err := ModelsPath()
	if err != nil {
		return fmt.Errorf("resolve models path: %w", err)
	}
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		if err := writeModels(modPath); err != nil {
			return fmt.Errorf("write models.yaml: %w", err)
		}
		fmt.Println("  models.yaml ✓")
	} else {
		fmt.Println("  models.yaml (exists, skipping)")
	}

	// Detect API keys
	candidates := detectAPIKeys(modelDefaults)

	var selectedModel string

	if len(candidates) == 0 {
		selectedModel = noKeysFlow(modelDefaults)
	} else {
		// Pick best available: detection order is anthropic, openai, google, ...
		// so candidates[0] is already the highest priority.
		best := candidates[0]
		fmt.Printf("Detected API keys:")
		for _, c := range candidates {
			fmt.Printf(" %s", c.provider)
		}
		fmt.Println()
		fmt.Printf("  Default provider: %s (%s)\n", best.provider, best.model)
		selectedModel = best.model
	}

	// Write config.yaml
	cfgPath, err := SettingsPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Println("  config.yaml (exists, skipping)")
	} else {
		if err := writeConfigYAML(cfgPath, selectedModel); err != nil {
			return fmt.Errorf("write config.yaml: %w", err)
		}
		fmt.Println("  config.yaml ✓")
	}

	fmt.Println()
	fmt.Println("Extensions will be installed automatically on first launch.")
	fmt.Println()
	fmt.Println("Setup complete! Run 'piglet' to start.")
	return nil
}

// providerEnvKeys maps provider name → env var names checked for an API key.
// Order determines setup priority (first detected wins).
// Default model IDs (e.g. claude-opus-4-6, gpt-5.4, gemini-3.1-pro-preview) are
// loaded at runtime from models.yaml via SetSetupModelDefaults.
var providerEnvKeys = []struct {
	provider string
	envKeys  []string
}{
	{"anthropic", []string{"ANTHROPIC_API_KEY"}},
	{"openai", []string{"OPENAI_API_KEY"}},
	{"google", []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}},
	{"xai", []string{"XAI_API_KEY"}},
	{"groq", []string{"GROQ_API_KEY"}},
	{"openrouter", []string{"OPENROUTER_API_KEY"}},
	{"zai", []string{"ZAI_API_KEY"}},
}

// detectAPIKeys checks environment variables for known provider keys.
func detectAPIKeys(modelDefaults map[string]string) []providerCandidate {
	var found []providerCandidate
	for _, c := range providerEnvKeys {
		for _, key := range c.envKeys {
			if os.Getenv(key) != "" {
				found = append(found, providerCandidate{
					provider: c.provider,
					envKey:   key,
					model:    setupDefaultModel(c.provider, modelDefaults),
				})
				break
			}
		}
	}
	return found
}

// setupDefaultModel returns the default model ID for a provider.
// Falls back to the provider name if modelDefaults is nil or the provider is absent.
func setupDefaultModel(provider string, modelDefaults map[string]string) string {
	if model, ok := modelDefaults[provider]; ok {
		return model
	}
	return provider
}

func noKeysFlow(modelDefaults map[string]string) string {
	fmt.Println("No API keys detected in environment.")
	fmt.Println()
	fmt.Println("Set an API key to get started:")
	fmt.Println("  export ANTHROPIC_API_KEY=sk-ant-...")
	fmt.Println("  export OPENAI_API_KEY=sk-...")
	fmt.Println("  export GOOGLE_API_KEY=AIza...")
	fmt.Println()
	fmt.Println("Or add it to ~/.config/piglet/auth.json:")
	fmt.Println(`  {"anthropic": "sk-ant-..."}`)
	fmt.Println()
	model := setupDefaultModel("anthropic", modelDefaults)
	fmt.Printf("Default model set to: %s (change in config.yaml)\n", model)
	return model
}

func writeConfigYAML(path, model string) error {
	return SaveTo(Settings{DefaultModel: model}, path)
}
