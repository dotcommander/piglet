// Package modelsdev syncs model metadata from https://models.dev into piglet's registry.
package modelsdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/provider"
)

const apiURL = "https://models.dev/api.json"

// supportedProviders lists provider names shared between models.dev and piglet.
var supportedProviders = []string{"anthropic", "google", "groq", "openai", "openrouter", "xai"}

// apiResponse is the top-level JSON object: provider name → provider data.
type apiResponse map[string]providerData

// providerData holds provider-level metadata and its model catalog.
type providerData struct {
	ID     string               `json:"id"`
	Models map[string]modelData `json:"models"`
}

// modelData holds per-model metadata from models.dev.
type modelData struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Limit modelLimit `json:"limit"`
}

// modelLimit holds token window/output limits.
type modelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// Sync fetches the models.dev catalog and updates existing models in the registry.
// Only providers with configured auth are processed. No new models are added.
// Returns the count of updated models, or an error.
func Sync(ctx context.Context, registry *provider.Registry, auth *config.Auth) (updated int, err error) {
	return SyncFromURL(ctx, apiURL, registry, auth)
}

// SyncFromURL is like Sync but fetches from the given URL instead of the default.
func SyncFromURL(ctx context.Context, url string, registry *provider.Registry, auth *config.Auth) (updated int, err error) {
	data, err := fetch(ctx, url)
	if err != nil {
		return 0, fmt.Errorf("modelsdev: fetch: %w", err)
	}

	// Index existing models by provider/id for quick lookup.
	existing := indexExisting(registry)

	for _, provName := range supportedProviders {
		if auth != nil && !auth.HasAuth(provName) {
			continue
		}

		prov, ok := data[provName]
		if !ok {
			continue
		}

		updated += updateProviderModels(registry, existing, provName, prov)
	}

	return updated, nil
}

// fetch retrieves and parses the models.dev API response from the given URL.
// Callers are responsible for setting a timeout on ctx.
func fetch(ctx context.Context, url string) (apiResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "piglet")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	return result, nil
}

// indexExisting builds a map from provider/id → core.Model for existing registry entries.
func indexExisting(registry *provider.Registry) map[string]core.Model {
	models := registry.Models()
	index := make(map[string]core.Model, len(models))
	for _, m := range models {
		index[m.Provider+"/"+m.ID] = m
	}
	return index
}

// updateProviderModels updates existing models from one provider with fresh metadata.
// Returns count of updated models. Does NOT add new models.
func updateProviderModels(registry *provider.Registry, existing map[string]core.Model, provName string, prov providerData) (updated int) {
	for _, md := range prov.Models {
		key := provName + "/" + md.ID
		cur, exists := existing[key]
		if !exists {
			continue // only update existing models
		}

		changed := false
		if md.Limit.Context > 0 && md.Limit.Context != cur.ContextWindow {
			cur.ContextWindow = md.Limit.Context
			changed = true
		}
		if md.Limit.Output > 0 && md.Limit.Output != cur.MaxTokens {
			cur.MaxTokens = md.Limit.Output
			changed = true
		}
		if md.Name != "" && md.Name != cur.Name {
			cur.Name = md.Name
			changed = true
		}

		if changed {
			registry.Register(cur)
			updated++
		}
	}
	return updated
}
