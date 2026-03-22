// Repomap extension binary. Provides a repository structure map injected into
// the system prompt. Communicates with the piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dotcommander/piglet/repomap"
	sdk "github.com/dotcommander/piglet/sdk/go"
	"gopkg.in/yaml.v3"
)

var rm *repomap.Map

func main() {
	e := sdk.New("repomap", "0.1.0")

	e.OnInit(func(x *sdk.Extension) {
		cfg := loadConfig()
		rm = repomap.New(x.CWD(), cfg)

		buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer buildCancel()
		if err := rm.Build(buildCtx); err != nil {
			x.Log("warn", "repomap initial build failed: "+err.Error())
		}

		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Repository Map",
			Content: rm.String(),
			Order:   15,
		})
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "repomap_refresh",
		Description: "Force rebuild the repository map after major file changes.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		PromptHint: "Rebuild the repository map after major file changes",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if rm == nil {
				return sdk.ErrorResult("repository map not initialized"), nil
			}
			if err := rm.Build(ctx); err != nil {
				return sdk.ErrorResult("rebuild failed: " + err.Error()), nil
			}
			return sdk.TextResult(rm.String()), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "repomap_show",
		Description: "Show the current repository structure map without rebuilding.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		PromptHint: "Show the current repository structure map",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if rm == nil {
				return sdk.TextResult("No repository map available."), nil
			}
			out := rm.String()
			if out == "" {
				return sdk.TextResult("No repository map available."), nil
			}
			return sdk.TextResult(out), nil
		},
	})

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "repomap-stale-check",
		Priority: 50,
		Events:   []string{"turn_end"},
		Handle: func(ctx context.Context, eventType string, data json.RawMessage) *sdk.Action {
			if rm == nil || !rm.Stale() {
				return nil
			}
			if err := rm.Build(ctx); err != nil {
				return nil
			}
			return nil
		},
	})

	e.Run()
}

// pigletConfig mirrors the relevant subset of ~/.config/piglet/config.yaml.
type pigletConfig struct {
	Repomap struct {
		MaxTokens      int `yaml:"maxTokens"`
		MaxTokensNoCtx int `yaml:"maxTokensNoCtx"`
	} `yaml:"repomap"`
}

// loadConfig reads repomap settings from ~/.config/piglet/config.yaml.
// Missing file or missing section uses defaults from repomap.DefaultConfig().
func loadConfig() repomap.Config {
	cfg := repomap.DefaultConfig()

	configDir, err := os.UserConfigDir()
	if err != nil {
		return cfg
	}

	data, err := os.ReadFile(filepath.Join(configDir, "piglet", "config.yaml"))
	if err != nil {
		return cfg
	}

	var pc pigletConfig
	if err := yaml.Unmarshal(data, &pc); err != nil {
		return cfg
	}

	if pc.Repomap.MaxTokens > 0 {
		cfg.MaxTokens = pc.Repomap.MaxTokens
	}
	if pc.Repomap.MaxTokensNoCtx > 0 {
		cfg.MaxTokensNoCtx = pc.Repomap.MaxTokensNoCtx
	}

	return cfg
}
