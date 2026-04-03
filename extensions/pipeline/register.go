package pipeline

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/prompt.md
var defaultPrompt string

const pipelinesDir = "pipelines"

// configDir is set during OnInit and used by tool/command handlers.
var configDir string

// Register registers the pipeline extension's tools and commands.
func Register(e *sdk.Extension) {
	e.OnInit(func(ext *sdk.Extension) {
		start := time.Now()
		ext.Log("debug", "[pipeline] OnInit start")

		home, err := xdg.ConfigDir()
		if err != nil {
			ext.Log("debug", fmt.Sprintf("[pipeline] OnInit complete — config dir error (%s)", time.Since(start)))
			return
		}
		configDir = home

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
		dir := filepath.Join(configDir, pipelinesDir)
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
	dir := filepath.Join(configDir, pipelinesDir)
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

func handlePipeCommand(ctx context.Context, e *sdk.Extension, args string) error {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		e.ShowMessage("Usage: /pipe <name> [--param key=value ...] [--dry-run]")
		return nil
	}

	name := parts[0]
	params := make(map[string]string)
	dryRun := false

	for i := 1; i < len(parts); i++ {
		switch {
		case parts[i] == "--dry-run":
			dryRun = true
		case parts[i] == "--param" && i+1 < len(parts):
			i++
			k, v, ok := strings.Cut(parts[i], "=")
			if ok {
				params[k] = v
			}
		}
	}

	dir := filepath.Join(configDir, pipelinesDir)
	path := filepath.Join(dir, name+".yaml")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(dir, name+".yml")
	}

	p, err := LoadFile(path)
	if err != nil {
		e.ShowMessage(fmt.Sprintf("Error loading pipeline %q: %s", name, err))
		return nil
	}

	e.ShowMessage(fmt.Sprintf("Running pipeline %q (%d steps)...", p.Name, len(p.Steps)))

	var result *PipelineResult
	if dryRun {
		result, err = DryRun(p, params)
	} else {
		result, err = Run(ctx, p, params)
	}
	if err != nil {
		e.ShowMessage(fmt.Sprintf("Pipeline error: %s", err))
		return nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Pipeline: %s\n\n", result.Name))
	for _, sr := range result.Steps {
		icon := "+"
		if sr.Status == "error" {
			icon = "x"
		} else if sr.Status == "skipped" {
			icon = "-"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s (%dms)\n", icon, sr.Name, sr.DurationMS))
		if sr.Output != "" {
			sb.WriteString(TruncateUTF8(sr.Output, 500) + "\n")
		}
		if sr.Error != "" {
			sb.WriteString(fmt.Sprintf("  error: %s\n", sr.Error))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf("**Result**: %s — %s", result.Status, result.Message))
	e.ShowMessage(sb.String())
	return nil
}

func handlePipeNewCommand(e *sdk.Extension, args string) error {
	name := strings.TrimSpace(args)
	if name == "" {
		e.ShowMessage("Usage: /pipe-new <name>")
		return nil
	}

	dir := filepath.Join(configDir, pipelinesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.ShowMessage(fmt.Sprintf("Error creating pipelines dir: %s", err))
		return nil
	}

	path := filepath.Join(dir, name+".yaml")
	if _, err := os.Stat(path); err == nil {
		e.ShowMessage(fmt.Sprintf("Pipeline %q already exists at %s", name, path))
		return nil
	}

	template := fmt.Sprintf(`name: %s
description: TODO — describe what this pipeline does
params:
  root:
    default: "."
    description: Root directory

steps:
  - name: hello
    run: echo "Hello from pipeline %s"
    description: A starter step

  - name: list-files
    run: ls -la {param.root} | head -10
    description: List files in root directory
`, name, name)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(template), 0o644); err != nil {
		e.ShowMessage(fmt.Sprintf("Error writing template: %s", err))
		return nil
	}
	if err := os.Rename(tmp, path); err != nil {
		e.ShowMessage(fmt.Sprintf("Error saving template: %s", err))
		os.Remove(tmp)
		return nil
	}

	e.ShowMessage(fmt.Sprintf("Created pipeline template at:\n%s\n\nEdit the file, then run: /pipe %s", path, name))
	return nil
}
