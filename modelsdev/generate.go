package modelsdev

import (
	"context"
	"strings"
	"time"

	"github.com/dotcommander/piglet/provider"
)

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
		return "", err
	}
	// Best-effort cache write — don't fail the generation if caching fails.
	_ = writeCache(&cache{FetchedAt: time.Now(), Data: data})
	return generateYAML(data), nil
}

// generateYAML builds models.yaml content from an API response,
// using provider.CuratedModels() as the single source of truth for
// the model list and provider.GenerateModelsYAML for YAML generation.
func generateYAML(data apiResponse) string {
	overrides := buildOverrides(data)
	return provider.GenerateModelsYAML(provider.CuratedModels(), overrides)
}

// buildOverrides extracts context/output/name values from the API response
// keyed by "provider/id" for models in the curated list.
func buildOverrides(data apiResponse) map[string]provider.CuratedModelOverride {
	// Index all API models by provider/id
	type apiModel struct {
		context int
		output  int
		name    string
	}
	index := make(map[string]apiModel)
	for provName, prov := range data {
		for _, md := range prov.Models {
			index[strings.ToLower(provName)+"/"+strings.ToLower(md.ID)] = apiModel{
				context: md.Limit.Context,
				output:  md.Limit.Output,
				name:    md.Name,
			}
		}
	}

	overrides := make(map[string]provider.CuratedModelOverride)
	for _, cm := range provider.CuratedModels() {
		key := strings.ToLower(cm.Provider) + "/" + strings.ToLower(cm.ID)
		if am, ok := index[key]; ok {
			overrides[key] = provider.CuratedModelOverride{
				Name:          am.name,
				ContextWindow: am.context,
				MaxTokens:     am.output,
			}
		}
	}
	return overrides
}
