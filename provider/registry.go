package provider

import (
	"cmp"
	"fmt"
	"os"
	"slices"
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
	if _, err := r.loadModels(); err != nil {
		// Non-fatal: registry starts empty, user gets "unknown model" errors
		fmt.Fprintf(os.Stderr, "warning: load models: %v\n", err)
	}
	return r
}

// NewRegistryFromData creates a registry from raw YAML model data without reading from disk.
func NewRegistryFromData(data []byte) (*Registry, error) {
	r := &Registry{models: make(map[string]core.Model)}
	if _, err := r.loadFromData(data); err != nil {
		return nil, fmt.Errorf("load models from data: %w", err)
	}
	return r, nil
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

	// Exact match: "openai/gpt-5"
	if m, ok := r.models[strings.ToLower(query)]; ok {
		return m, true
	}

	// Single pass: collect exact-ID, prefix, and substring candidates simultaneously,
	// then return the highest-priority match. m.ID is lowercased once per model.
	lower := strings.ToLower(query)
	var exactID, prefix, substring core.Model
	hasExact, hasPrefix, hasSub := false, false, false
	for _, m := range r.models {
		ml := strings.ToLower(m.ID)
		if ml == lower {
			exactID, hasExact = m, true
			break // exact ID match wins — no need to scan further
		}
		if strings.HasPrefix(ml, lower) && (!hasPrefix || len(m.ID) < len(prefix.ID)) {
			prefix, hasPrefix = m, true
			continue
		}
		if !hasSub || len(m.ID) < len(substring.ID) {
			if strings.Contains(ml, lower) || strings.Contains(strings.ToLower(m.Name), lower) {
				substring, hasSub = m, true
			}
		}
	}
	switch {
	case hasExact:
		return exactID, true
	case hasPrefix:
		return prefix, true
	case hasSub:
		return substring, true
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
	slices.SortFunc(models, func(a, b core.Model) int {
		if a.Provider != b.Provider {
			return cmp.Compare(a.Provider, b.Provider)
		}
		return cmp.Compare(a.ID, b.ID)
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
	slices.SortFunc(models, func(a, b core.Model) int { return cmp.Compare(a.ID, b.ID) })
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

func (r *Registry) loadModels() (int, error) {
	path, err := config.ModelsPath()
	if err != nil {
		return 0, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			data = []byte(DefaultModelsYAML())
		} else {
			return 0, fmt.Errorf("read models: %w", err)
		}
	}

	return r.loadFromData(data)
}

func (r *Registry) loadFromData(data []byte) (int, error) {
	var file modelsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return 0, fmt.Errorf("parse models.yaml: %w", err)
	}

	built := make(map[string]core.Model, len(file.Models))
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
		built[modelKey(m.Provider, m.ID)] = m
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for k, m := range built {
		r.models[k] = m
	}

	return len(file.Models), nil
}

// ReloadModels re-reads models.yaml from disk, replacing in-memory entries.
func (r *Registry) ReloadModels() (int, error) {
	return r.loadModels()
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
