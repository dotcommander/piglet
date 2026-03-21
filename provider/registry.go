package provider

import (
	"github.com/dotcommander/piglet/core"
	"sort"
	"strings"
	"sync"
)

// Registry holds the model catalog and creates providers.
type Registry struct {
	mu     sync.RWMutex
	models map[string]core.Model // key = "provider/model-id"
}

// NewRegistry creates a registry with built-in models.
func NewRegistry() *Registry {
	r := &Registry{models: make(map[string]core.Model)}
	r.registerBuiltins()
	return r
}

// Register adds a model to the registry.
func (r *Registry) Register(m core.Model) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[modelKey(m.Provider, m.ID)] = m
}

// Resolve finds a model by provider/id string or just model id.
func (r *Registry) Resolve(query string) (core.Model, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Exact match: "openai/gpt-4o"
	if m, ok := r.models[strings.ToLower(query)]; ok {
		return m, true
	}

	// Search by model ID across all providers
	lower := strings.ToLower(query)
	for _, m := range r.models {
		if strings.ToLower(m.ID) == lower {
			return m, true
		}
	}

	// Prefix match
	for _, m := range r.models {
		if strings.HasPrefix(strings.ToLower(m.ID), lower) {
			return m, true
		}
	}

	return core.Model{}, false
}

// Models returns all registered models sorted by provider then ID.
func (r *Registry) Models() []core.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]core.Model, 0, len(r.models))
	for _, m := range r.models {
		models = append(models, m)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})
	return models
}

// ModelsByProvider returns models for a specific provider.
func (r *Registry) ModelsByProvider(provider string) []core.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lower := strings.ToLower(provider)
	var models []core.Model
	for _, m := range r.models {
		if strings.ToLower(m.Provider) == lower {
			models = append(models, m)
		}
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models
}

// Create creates a StreamProvider for the given model.
func (r *Registry) Create(m core.Model, apiKeyFn func() string) (core.StreamProvider, error) {
	switch m.API {
	case core.APIOpenAI:
		return NewOpenAI(m, apiKeyFn), nil
	case core.APIAnthropic:
		return NewAnthropic(m, apiKeyFn), nil
	case core.APIGoogle:
		return NewGoogle(m, apiKeyFn), nil
	default:
		// Default to OpenAI-compatible
		return NewOpenAI(m, apiKeyFn), nil
	}
}

func modelKey(provider, id string) string {
	return strings.ToLower(provider) + "/" + strings.ToLower(id)
}

func (r *Registry) registerBuiltins() {
	builtins := []core.Model{
		// OpenAI
		{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", API: core.APIOpenAI, BaseURL: "https://api.openai.com", ContextWindow: 128000, MaxTokens: 16384},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Provider: "openai", API: core.APIOpenAI, BaseURL: "https://api.openai.com", ContextWindow: 128000, MaxTokens: 16384},
		{ID: "o3-mini", Name: "o3-mini", Provider: "openai", API: core.APIOpenAI, BaseURL: "https://api.openai.com", ContextWindow: 200000, MaxTokens: 100000},

		// Anthropic
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", API: core.APIAnthropic, BaseURL: "https://api.anthropic.com", ContextWindow: 200000, MaxTokens: 8192},
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", Provider: "anthropic", API: core.APIAnthropic, BaseURL: "https://api.anthropic.com", ContextWindow: 200000, MaxTokens: 8192},
		{ID: "claude-haiku-3-5-20241022", Name: "Claude 3.5 Haiku", Provider: "anthropic", API: core.APIAnthropic, BaseURL: "https://api.anthropic.com", ContextWindow: 200000, MaxTokens: 8192},

		// Google
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Provider: "google", API: core.APIGoogle, BaseURL: "https://generativelanguage.googleapis.com", ContextWindow: 1048576, MaxTokens: 65536},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Provider: "google", API: core.APIGoogle, BaseURL: "https://generativelanguage.googleapis.com", ContextWindow: 1048576, MaxTokens: 65536},

		// xAI
		{ID: "grok-3", Name: "Grok 3", Provider: "xai", API: core.APIOpenAI, BaseURL: "https://api.x.ai", ContextWindow: 131072, MaxTokens: 16384},

		// Groq
		{ID: "llama-3.3-70b-versatile", Name: "Llama 3.3 70B", Provider: "groq", API: core.APIOpenAI, BaseURL: "https://api.groq.com/openai", ContextWindow: 131072, MaxTokens: 8192},

		// OpenRouter
		{ID: "auto", Name: "Auto", Provider: "openrouter", API: core.APIOpenAI, BaseURL: "https://openrouter.ai/api", ContextWindow: 200000, MaxTokens: 16384},
	}

	for _, m := range builtins {
		r.models[modelKey(m.Provider, m.ID)] = m
	}
}
