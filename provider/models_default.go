package provider

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultModelsYAML = `models:
  # Anthropic
  - id: claude-opus-4-6
    name: Claude Opus 4.6
    provider: anthropic
    api: anthropic
    baseUrl: https://api.anthropic.com
    contextWindow: 1000000
    maxTokens: 128000

  - id: claude-sonnet-4-6
    name: Claude Sonnet 4.6
    provider: anthropic
    api: anthropic
    baseUrl: https://api.anthropic.com
    contextWindow: 1000000
    maxTokens: 64000

  - id: claude-sonnet-4-20250514
    name: Claude Sonnet 4
    provider: anthropic
    api: anthropic
    baseUrl: https://api.anthropic.com
    contextWindow: 200000
    maxTokens: 64000

  - id: claude-haiku-4-5-20251001
    name: Claude Haiku 4.5
    provider: anthropic
    api: anthropic
    baseUrl: https://api.anthropic.com
    contextWindow: 200000
    maxTokens: 64000

  # OpenAI
  - id: gpt-5.4
    name: GPT-5.4
    provider: openai
    api: openai
    baseUrl: https://api.openai.com
    contextWindow: 1050000
    maxTokens: 128000

  - id: gpt-5
    name: GPT-5
    provider: openai
    api: openai
    baseUrl: https://api.openai.com
    contextWindow: 400000
    maxTokens: 128000

  - id: o4-mini
    name: o4-mini
    provider: openai
    api: openai
    baseUrl: https://api.openai.com
    contextWindow: 200000
    maxTokens: 100000

  - id: gpt-4.1
    name: GPT-4.1
    provider: openai
    api: openai
    baseUrl: https://api.openai.com
    contextWindow: 1047576
    maxTokens: 32768

  - id: gpt-4.1-mini
    name: GPT-4.1 mini
    provider: openai
    api: openai
    baseUrl: https://api.openai.com
    contextWindow: 1047576
    maxTokens: 32768

  - id: gpt-4o
    name: GPT-4o
    provider: openai
    api: openai
    baseUrl: https://api.openai.com
    contextWindow: 128000
    maxTokens: 16384

  - id: o3
    name: o3
    provider: openai
    api: openai
    baseUrl: https://api.openai.com
    contextWindow: 200000
    maxTokens: 100000

  # Google
  - id: gemini-3.1-pro-preview
    name: Gemini 3.1 Pro Preview
    provider: google
    api: google
    baseUrl: https://generativelanguage.googleapis.com
    contextWindow: 1048576
    maxTokens: 65536

  - id: gemini-2.5-pro
    name: Gemini 2.5 Pro
    provider: google
    api: google
    baseUrl: https://generativelanguage.googleapis.com
    contextWindow: 1048576
    maxTokens: 65536

  - id: gemini-2.5-flash
    name: Gemini 2.5 Flash
    provider: google
    api: google
    baseUrl: https://generativelanguage.googleapis.com
    contextWindow: 1048576
    maxTokens: 65536

  # xAI
  - id: grok-3
    name: Grok 3
    provider: xai
    api: openai
    baseUrl: https://api.x.ai
    contextWindow: 131072
    maxTokens: 8192

  # Groq (free/fast inference)
  - id: llama-3.3-70b-versatile
    name: Llama 3.3 70B
    provider: groq
    api: openai
    baseUrl: https://api.groq.com/openai
    contextWindow: 131072
    maxTokens: 32768

  # OpenRouter
  - id: auto
    name: Auto (best available)
    provider: openrouter
    api: openai
    baseUrl: https://openrouter.ai/api
    contextWindow: 200000
    maxTokens: 16384

  # Z.AI (GLM models)
  - id: glm-5
    name: GLM-5
    provider: zai
    api: openai
    baseUrl: https://api.z.ai/api/paas/v4
    contextWindow: 128000
    maxTokens: 8192

  - id: glm-4.7
    name: GLM-4.7
    provider: zai
    api: openai
    baseUrl: https://api.z.ai/api/paas/v4
    contextWindow: 128000
    maxTokens: 8192

  - id: glm-5-turbo
    name: GLM-5 Turbo
    provider: zai
    api: openai
    baseUrl: https://api.z.ai/api/paas/v4
    contextWindow: 128000
    maxTokens: 8192

  # LM Studio (local)
  - id: local-model
    name: Local Model
    provider: lmstudio
    api: openai
    baseUrl: http://localhost:1234
    contextWindow: 32000
    maxTokens: 32000
`

// DefaultModelsYAML returns the raw default models catalog YAML.
func DefaultModelsYAML() string {
	return defaultModelsYAML
}

// WriteDefaultModels writes the default models catalog to path.
// Delegates to WriteModelsData with the hardcoded default.
func WriteDefaultModels(path string) error {
	return WriteModelsData(path, defaultModelsYAML)
}

// WriteModelsData writes the given YAML content to path using an atomic write
// (temp file + rename). It creates the parent directory if needed.
// Returns nil without writing if the file already exists.
func WriteModelsData(path string, data string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o600); err != nil {
		return fmt.Errorf("write models data: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install models data: %w", err)
	}

	return nil
}
