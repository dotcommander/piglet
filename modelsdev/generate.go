package modelsdev

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// curatedModel defines a model to include in the generated catalog.
type curatedModel struct {
	id       string
	name     string
	provider string // models.dev provider key
	api      string // piglet API type
	baseURL  string
}

// curatedModels is the selection list — same models as provider/models_default.go.
var curatedModels = []curatedModel{
	// Anthropic
	{id: "claude-opus-4-6", name: "Claude Opus 4.6", provider: "anthropic", api: "anthropic", baseURL: "https://api.anthropic.com"},
	{id: "claude-sonnet-4-6", name: "Claude Sonnet 4.6", provider: "anthropic", api: "anthropic", baseURL: "https://api.anthropic.com"},
	{id: "claude-sonnet-4-20250514", name: "Claude Sonnet 4", provider: "anthropic", api: "anthropic", baseURL: "https://api.anthropic.com"},
	{id: "claude-haiku-4-5-20251001", name: "Claude Haiku 4.5", provider: "anthropic", api: "anthropic", baseURL: "https://api.anthropic.com"},
	// OpenAI
	{id: "gpt-5.4", name: "GPT-5.4", provider: "openai", api: "openai", baseURL: "https://api.openai.com"},
	{id: "gpt-5", name: "GPT-5", provider: "openai", api: "openai", baseURL: "https://api.openai.com"},
	{id: "o4-mini", name: "o4-mini", provider: "openai", api: "openai", baseURL: "https://api.openai.com"},
	{id: "gpt-4.1", name: "GPT-4.1", provider: "openai", api: "openai", baseURL: "https://api.openai.com"},
	{id: "gpt-4.1-mini", name: "GPT-4.1 mini", provider: "openai", api: "openai", baseURL: "https://api.openai.com"},
	{id: "gpt-4o", name: "GPT-4o", provider: "openai", api: "openai", baseURL: "https://api.openai.com"},
	{id: "o3", name: "o3", provider: "openai", api: "openai", baseURL: "https://api.openai.com"},
	// Google
	{id: "gemini-3.1-pro-preview", name: "Gemini 3.1 Pro Preview", provider: "google", api: "google", baseURL: "https://generativelanguage.googleapis.com"},
	{id: "gemini-2.5-pro", name: "Gemini 2.5 Pro", provider: "google", api: "google", baseURL: "https://generativelanguage.googleapis.com"},
	{id: "gemini-2.5-flash", name: "Gemini 2.5 Flash", provider: "google", api: "google", baseURL: "https://generativelanguage.googleapis.com"},
	// xAI
	{id: "grok-3", name: "Grok 3", provider: "xai", api: "openai", baseURL: "https://api.x.ai"},
	// Groq
	{id: "llama-3.3-70b-versatile", name: "Llama 3.3 70B", provider: "groq", api: "openai", baseURL: "https://api.groq.com/openai"},
	// OpenRouter
	{id: "auto", name: "Auto (best available)", provider: "openrouter", api: "openai", baseURL: "https://openrouter.ai/api"},
	// Z.AI
	{id: "glm-5", name: "GLM-5", provider: "zai", api: "openai", baseURL: "https://api.z.ai/api/coding/paas/v4"},
	{id: "glm-4.7", name: "GLM-4.7", provider: "zai", api: "openai", baseURL: "https://api.z.ai/api/coding/paas/v4"},
	{id: "glm-5-turbo", name: "GLM-5 Turbo", provider: "zai", api: "openai", baseURL: "https://api.z.ai/api/coding/paas/v4"},
	// LM Studio (local — not on models.dev)
	{id: "local-model", name: "Local Model", provider: "lmstudio", api: "openai", baseURL: "http://localhost:1234"},
}

// GenerateModelsYAML fetches the models.dev API and generates a models.yaml
// string with current context window and max token values for the curated set.
func GenerateModelsYAML(ctx context.Context) (string, error) {
	return GenerateModelsYAMLFromURL(ctx, apiURL)
}

// GenerateModelsYAMLFromURL fetches from the given URL and generates models YAML.
// As a side effect, it writes the API response to the local cache so that
// background refresh won't redundantly re-fetch.
func GenerateModelsYAMLFromURL(ctx context.Context, url string) (string, error) {
	data, err := fetch(ctx, url)
	if err != nil {
		return "", fmt.Errorf("modelsdev: fetch: %w", err)
	}
	// Best-effort cache write — don't fail the generation if caching fails.
	_ = writeCache(&cache{FetchedAt: time.Now(), Data: data})
	return generateYAML(data), nil
}

// generateYAML builds models.yaml content from an API response.
// Models not found in the API use sensible defaults (32000 context/tokens).
func generateYAML(data apiResponse) string {
	type apiModel struct {
		context int
		output  int
		name    string
	}
	index := make(map[string]apiModel)
	for provName, prov := range data {
		for _, md := range prov.Models {
			index[provName+"/"+md.ID] = apiModel{
				context: md.Limit.Context,
				output:  md.Limit.Output,
				name:    md.Name,
			}
		}
	}

	var b strings.Builder
	b.WriteString("models:\n")

	prevProvider := ""
	for _, cm := range curatedModels {
		if cm.provider != prevProvider {
			fmt.Fprintf(&b, "  # %s\n", providerLabel(cm.provider))
			prevProvider = cm.provider
		}

		contextWindow := 32000
		maxTokens := 32000
		name := cm.name

		if am, ok := index[cm.provider+"/"+cm.id]; ok {
			if am.context > 0 {
				contextWindow = am.context
			}
			if am.output > 0 {
				maxTokens = am.output
			}
			if am.name != "" {
				name = am.name
			}
		}

		fmt.Fprintf(&b, "  - id: %s\n", cm.id)
		fmt.Fprintf(&b, "    name: %s\n", name)
		fmt.Fprintf(&b, "    provider: %s\n", cm.provider)
		fmt.Fprintf(&b, "    api: %s\n", cm.api)
		fmt.Fprintf(&b, "    baseUrl: %s\n", cm.baseURL)
		fmt.Fprintf(&b, "    contextWindow: %d\n", contextWindow)
		fmt.Fprintf(&b, "    maxTokens: %d\n", maxTokens)
		b.WriteString("\n")
	}

	return b.String()
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
