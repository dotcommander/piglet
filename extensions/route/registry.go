package route

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
	"gopkg.in/yaml.v3"
)

// ComponentType identifies what kind of piglet component this is.
type ComponentType string

const (
	TypeExtension ComponentType = "extension"
	TypeTool      ComponentType = "tool"
	TypeCommand   ComponentType = "command"
)

// Component is a scored entry in the routing registry.
type Component struct {
	Name         string        // e.g. "safeguard", "dispatch", "/plan"
	Type         ComponentType // extension, tool, or command
	Extension    string        // parent extension name (empty for extension-level entries)
	Description  string        // from tool description or manifest
	PromptHint   string        // from ToolDef.PromptHint
	Keywords     []string      // extracted from name + description
	Triggers     []string      // multi-word trigger phrases
	Intents      []string      // declared intents from manifest (e.g. "debug", "test")
	Domains      []string      // declared domains from manifest (e.g. "go", "security")
	AntiTriggers []string      // tokens that penalize this component's score
}

// Registry holds all discoverable piglet components for scoring.
type Registry struct {
	Components []Component
}

// BuildRegistry queries the host for loaded extensions and tools, then
// enriches with manifest metadata from disk.
func BuildRegistry(ctx context.Context, ext *sdk.Extension) (*Registry, error) {
	reg := &Registry{}

	// Get extension metadata from host
	extInfos, err := ext.ExtInfos(ctx)
	if err != nil {
		return reg, err
	}

	// Get tool descriptions from host
	hostTools, err := ext.ListHostTools(ctx, "all")
	if err != nil {
		return reg, err
	}

	toolDescriptions := make(map[string]sdk.HostToolInfo, len(hostTools))
	for _, t := range hostTools {
		toolDescriptions[t.Name] = t
	}

	// Get extensions directory for manifest scanning
	extDir, _ := ext.ExtensionsDir(ctx)

	for _, info := range extInfos {
		if info.Name == "route" {
			continue // skip self
		}

		// Load manifest for richer metadata
		manifest := loadManifest(extDir, info.Name)

		// Register extension-level component
		extComp := Component{
			Name:         info.Name,
			Type:         TypeExtension,
			Keywords:     extractNameKeywords(info.Name),
			Triggers:     manifest.Triggers,
			Intents:      manifest.Intents,
			Domains:      manifest.Domains,
			AntiTriggers: manifest.AntiTriggers,
			Extension:    info.Name,
		}
		if manifest.Description != "" {
			extComp.Description = manifest.Description
			extComp.Keywords = append(extComp.Keywords, Tokenize(manifest.Description)...)
		}
		reg.Components = append(reg.Components, extComp)

		// Register each tool — inherit parent extension's intents/domains
		for _, toolName := range info.Tools {
			tc := Component{
				Name:         toolName,
				Type:         TypeTool,
				Extension:    info.Name,
				Keywords:     extractNameKeywords(toolName),
				Intents:      manifest.Intents,
				Domains:      manifest.Domains,
				AntiTriggers: manifest.AntiTriggers,
			}
			if ht, ok := toolDescriptions[toolName]; ok {
				tc.Description = ht.Description
				tc.Keywords = append(tc.Keywords, Tokenize(ht.Description)...)
			}
			reg.Components = append(reg.Components, tc)
		}

		// Register each command — inherit parent extension's intents/domains
		for _, cmdName := range info.Commands {
			reg.Components = append(reg.Components, Component{
				Name:         "/" + cmdName,
				Type:         TypeCommand,
				Extension:    info.Name,
				Keywords:     extractNameKeywords(cmdName),
				Intents:      manifest.Intents,
				Domains:      manifest.Domains,
				AntiTriggers: manifest.AntiTriggers,
			})
		}
	}

	reg.dedup()
	return reg, nil
}

// manifestMeta is the subset of manifest.yaml we care about for routing.
type manifestMeta struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Triggers     []string `yaml:"triggers"`
	Intents      []string `yaml:"intents"`
	Domains      []string `yaml:"domains"`
	AntiTriggers []string `yaml:"anti_triggers"`
}

func loadManifest(extDir, name string) manifestMeta {
	if extDir == "" {
		return manifestMeta{}
	}
	path := filepath.Join(extDir, name, "manifest.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return manifestMeta{}
	}
	var m manifestMeta
	_ = yaml.Unmarshal(data, &m)
	return m
}

// extractNameKeywords splits a component name on underscores and hyphens
// to produce keyword tokens. "skill_load" -> ["skill", "load"].
func extractNameKeywords(name string) []string {
	name = strings.ToLower(name)
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == '/'
	})
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) > 1 && !stopWords[p] {
			result = append(result, p)
		}
	}
	return result
}

// dedup removes duplicate keywords within each component.
func (r *Registry) dedup() {
	for i := range r.Components {
		r.Components[i].Keywords = dedupStrings(r.Components[i].Keywords)
	}
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
