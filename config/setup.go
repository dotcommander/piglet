package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	switch len(candidates) {
	case 0:
		selectedModel = noKeysFlow()

	case 1:
		selectedModel = singleKeyFlow(candidates[0])

	default:
		var err error
		selectedModel, err = multiKeyFlow(candidates)
		if err != nil {
			return fmt.Errorf("provider selection: %w", err)
		}
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
	fmt.Println("Extensions add memory, skills, code intelligence, and more.")
	fmt.Println("Install: https://github.com/dotcommander/piglet-extensions")
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
		{[]string{"ANTHROPIC_API_KEY"}, "anthropic", "claude-sonnet-4-20250514"},
		{[]string{"OPENAI_API_KEY"}, "openai", "gpt-4.1"},
		{[]string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}, "google", "gemini-2.5-flash"},
		{[]string{"XAI_API_KEY"}, "xai", "grok-3"},
		{[]string{"GROQ_API_KEY"}, "groq", "llama-3.3-70b-versatile"},
		{[]string{"OPENROUTER_API_KEY"}, "openrouter", "auto"},
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

func singleKeyFlow(c providerCandidate) string {
	fmt.Printf("  Detected %s\n", c.envKey)
	fmt.Printf("  Default model: %s\n", c.model)
	return c.model
}

func noKeysFlow() string {
	fmt.Println("No API keys detected in environment.")
	fmt.Println("Set one of: ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY")
	fmt.Println()
	model := "claude-sonnet-4-20250514"
	fmt.Printf("Default model set to: %s\n", model)
	fmt.Printf("You can change this in ~/.config/piglet/config.yaml\n")
	return model
}

func multiKeyFlow(candidates []providerCandidate) (string, error) {
	fmt.Println("Detected API keys:")
	for i, c := range candidates {
		fmt.Printf("  [%d] %s (%s)\n", i+1, c.provider, c.envKey)
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Select default provider [1]: ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}

	input := strings.TrimSpace(line)
	if input == "" {
		return candidates[0].model, nil
	}

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(candidates) {
		return "", fmt.Errorf("invalid selection %q: must be 1-%d", input, len(candidates))
	}

	chosen := candidates[n-1]
	fmt.Printf("  Default model: %s\n", chosen.model)
	return chosen.model, nil
}

func writeConfigYAML(path, model string) error {
	return SaveTo(Settings{DefaultModel: model}, path)
}
