package pipeline

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/prompt.md
var defaultPrompt string

const pipelinesDir = "pipelines"

// configDir returns the piglet config directory, resolved once on first call.
var configDir = sync.OnceValue(func() string {
	home, _ := xdg.ConfigDir()
	return home
})

// Register registers the pipeline extension's tools and commands.
func Register(e *sdk.Extension) {
	e.OnInit(func(ext *sdk.Extension) {
		start := time.Now()
		ext.Log("debug", "[pipeline] OnInit start")

		content := xdg.LoadOrCreateExt("pipeline", "prompt.md", strings.TrimSpace(defaultPrompt))
		if content != "" {
			ext.RegisterPromptSection(sdk.PromptSectionDef{
				Title:   "Pipelines",
				Content: content,
				Order:   75,
			})
		}

		ext.Log("debug", fmt.Sprintf("[pipeline] OnInit complete (%s)", time.Since(start)))
	})

	e.RegisterTool(sdk.ToolDef{
		Name:              "pipeline",
		Description:       "Run a multi-step workflow. Load a saved pipeline by name from ~/.config/piglet/pipelines/ or provide an inline pipeline definition. Steps execute sequentially with output passing, retries, loops, and error handling.",
		Deferred:          true,
		InterruptBehavior: "block",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name of a saved pipeline (loads from ~/.config/piglet/pipelines/<name>.yaml)",
				},
				"inline": map[string]any{
					"type":        "object",
					"description": "Ad-hoc pipeline definition. Same schema as YAML: name, description, steps[], params{}.",
				},
				"params": map[string]any{
					"type":        "object",
					"description": "Parameter overrides as key-value pairs.",
					"additionalProperties": map[string]any{
						"type": "string",
					},
				},
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Preview all steps without executing (default: false).",
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			return executePipeline(ctx, args)
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "pipeline_list",
		Description: "List all saved pipelines in ~/.config/piglet/pipelines/.",
		Deferred:    true,
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			return listPipelines()
		},
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "pipe",
		Description: "Run a saved pipeline: /pipe <name> [--param key=value ...] [--dry-run]",
		Handler: func(ctx context.Context, args string) error {
			return handlePipeCommand(ctx, e, args)
		},
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "pipe-new",
		Description: "Create a new pipeline template: /pipe-new <name>",
		Handler: func(ctx context.Context, args string) error {
			return handlePipeNewCommand(e, args)
		},
	})
}

func executePipeline(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
	params := make(map[string]string)
	if raw, ok := args["params"].(map[string]any); ok {
		for k, v := range raw {
			params[k] = fmt.Sprint(v)
		}
	}
	dryRun, _ := args["dry_run"].(bool)

	var p *Pipeline

	if name, ok := args["name"].(string); ok && name != "" {
		dir := filepath.Join(configDir(), pipelinesDir)
		path := filepath.Join(dir, name+".yaml")
		if _, err := os.Stat(path); err != nil {
			path = filepath.Join(dir, name+".yml")
		}
		var err error
		p, err = LoadFile(path)
		if err != nil {
			return sdk.ErrorResult(fmt.Sprintf("load pipeline %q: %s", name, err)), nil
		}
	} else if inline, ok := args["inline"].(map[string]any); ok {
		data, err := json.Marshal(inline)
		if err != nil {
			return sdk.ErrorResult(fmt.Sprintf("marshal inline pipeline: %s", err)), nil
		}
		p = new(Pipeline)
		if err := json.Unmarshal(data, p); err != nil {
			return sdk.ErrorResult(fmt.Sprintf("parse inline pipeline: %s", err)), nil
		}
	} else {
		return sdk.ErrorResult("provide either 'name' (saved pipeline) or 'inline' (ad-hoc definition)"), nil
	}

	var result *PipelineResult
	var err error

	if dryRun {
		result, err = DryRun(p, params)
	} else {
		result, err = Run(ctx, p, params)
	}
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("pipeline error: %s", err)), nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("marshal result: %s", err)), nil
	}
	return sdk.TextResult(string(data)), nil
}

func listPipelines() (*sdk.ToolResult, error) {
	dir := filepath.Join(configDir(), pipelinesDir)
	pipes, err := LoadDir(dir)
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("list pipelines: %s", err)), nil
	}
	if len(pipes) == 0 {
		return sdk.TextResult("No pipelines found in " + dir), nil
	}

	type entry struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		StepCount   int      `json:"step_count"`
		Params      []string `json:"params,omitempty"`
	}

	entries := make([]entry, len(pipes))
	for i, p := range pipes {
		var paramNames []string
		for name := range p.Params {
			paramNames = append(paramNames, name)
		}
		entries[i] = entry{
			Name:        p.Name,
			Description: p.Description,
			StepCount:   len(p.Steps),
			Params:      paramNames,
		}
	}

	data, _ := json.Marshal(entries)
	return sdk.TextResult(string(data)), nil
}
