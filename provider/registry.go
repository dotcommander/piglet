package provider

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"gopkg.in/yaml.v3"
)

// Registry holds the model catalog and creates providers.
type Registry struct {
	mu     sync.RWMutex
	models map[string]core.Model // key = "provider/model-id"
}

// NewRegistry creates a registry, loading models from ~/.config/piglet/models.yaml.
func NewRegistry() *Registry {
	r := &Registry{models: make(map[string]core.Model)}
	if err := r.loadModels(); err != nil {
		// Non-fatal: registry starts empty, user gets "unknown model" errors
		fmt.Fprintf(os.Stderr, "warning: load models: %v\n", err)
	}
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

	// Exact match: "openai/gpt-5.1"
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

// ---------------------------------------------------------------------------
// models.yaml loader
// ---------------------------------------------------------------------------

type modelsFile struct {
	Models []modelEntry `yaml:"models"`
}

type modelEntry struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	Provider      string `yaml:"provider"`
	API           string `yaml:"api"`
	BaseURL       string `yaml:"baseUrl"`
	ContextWindow int    `yaml:"contextWindow"`
	MaxTokens     int    `yaml:"maxTokens"`
}

func (r *Registry) loadModels() error {
	path, err := config.ModelsPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("models.yaml not found at %s — run: piglet init", path)
		}
		return fmt.Errorf("read models: %w", err)
	}

	var file modelsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse models.yaml: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, e := range file.Models {
		m := core.Model{
			ID:            e.ID,
			Name:          e.Name,
			Provider:      e.Provider,
			API:           parseAPI(e.API),
			BaseURL:       e.BaseURL,
			ContextWindow: e.ContextWindow,
			MaxTokens:     e.MaxTokens,
		}
		r.models[modelKey(m.Provider, m.ID)] = m
	}

	return nil
}

func parseAPI(s string) core.API {
	switch strings.ToLower(s) {
	case "anthropic":
		return core.APIAnthropic
	case "google":
		return core.APIGoogle
	default:
		return core.APIOpenAI
	}
}

