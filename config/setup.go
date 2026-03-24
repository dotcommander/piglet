package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// providerCandidate holds detected provider info from environment.
type providerCandidate struct {
	provider string
	envKey   string
	model    string
}

// ModelsPath returns the path to models.yaml in the config dir.
func ModelsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "models.yaml"), nil
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
func RunSetup(writeModels func(path string) error) error {
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
	modPath := filepath.Join(dir, "models.yaml")
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		if err := writeModels(modPath); err != nil {
			return fmt.Errorf("write models.yaml: %w", err)
		}
		fmt.Println("  models.yaml ✓")
	} else {
		fmt.Println("  models.yaml (exists, skipping)")
	}

	// Detect API keys
	candidates := detectAPIKeys()

	var selectedModel string

	if len(candidates) == 0 {
		selectedModel = noKeysFlow()
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
	cfgPath := filepath.Join(dir, "config.yaml")
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

// detectAPIKeys checks environment variables for known provider keys.
func detectAPIKeys() []providerCandidate {
	checks := []struct {
		envKeys  []string
		provider string
		model    string
	}{
		{[]string{"ANTHROPIC_API_KEY"}, "anthropic", "claude-opus-4-6"},
		{[]string{"OPENAI_API_KEY"}, "openai", "gpt-5.4"},
		{[]string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}, "google", "gemini-3.1-pro-preview"},
		{[]string{"XAI_API_KEY"}, "xai", "grok-3"},
		{[]string{"GROQ_API_KEY"}, "groq", "llama-3.3-70b-versatile"},
		{[]string{"OPENROUTER_API_KEY"}, "openrouter", "auto"},
		{[]string{"ZAI_API_KEY"}, "zai", "glm-5"},
	}

	var found []providerCandidate
	for _, c := range checks {
		for _, key := range c.envKeys {
			if os.Getenv(key) != "" {
				found = append(found, providerCandidate{
					provider: c.provider,
					envKey:   key,
					model:    c.model,
				})
				break
			}
		}
	}
	return found
}

func noKeysFlow() string {
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
	model := "claude-opus-4-6"
	fmt.Printf("Default model set to: %s (change in config.yaml)\n", model)
	return model
}

func writeConfigYAML(path, model string) error {
	return SaveTo(Settings{DefaultModel: model}, path)
}
