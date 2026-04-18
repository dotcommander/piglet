package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

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

	dir := filepath.Join(configDir(), pipelinesDir)
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

	dir := filepath.Join(configDir(), pipelinesDir)
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
