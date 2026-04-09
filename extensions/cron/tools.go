package cron

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	sdk "github.com/dotcommander/piglet/sdk"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

func registerTools(e *sdk.Extension) {
	e.RegisterTool(sdk.ToolDef{
		Name:        "cron_list",
		Description: "List all scheduled cron tasks with their schedules, last run, and next run times",
		Deferred:    true,
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, _ map[string]any) (*sdk.ToolResult, error) {
			summaries, err := ListTasks()
			if err != nil {
				return sdk.ErrorResult("Error: " + err.Error()), nil
			}
			if len(summaries) == 0 {
				return sdk.TextResult("No scheduled tasks configured."), nil
			}
			return sdk.TextResult(formatTaskList(summaries, false)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "cron_history",
		Description: "Show recent cron task execution history",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "Filter history by task name (optional)",
				},
				"limit": map[string]any{
					"type":        "number",
					"description": "Max entries to return (default 20)",
				},
			},
		},
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			entries, err := ReadHistory()
			if err != nil {
				return sdk.ErrorResult("Error: " + err.Error()), nil
			}

			taskFilter, _ := args["task"].(string)
			limit := 20
			if l, ok := args["limit"].(float64); ok && l > 0 {
				limit = int(l)
			}

			if taskFilter != "" {
				var filtered []RunEntry
				for _, en := range entries {
					if en.Task == taskFilter {
						filtered = append(filtered, en)
					}
				}
				entries = filtered
			}

			if len(entries) == 0 {
				return sdk.TextResult("No history found."), nil
			}

			start := 0
			if len(entries) > limit {
				start = len(entries) - limit
			}

			var b strings.Builder
			for _, entry := range entries[start:] {
				b.WriteString(formatHistoryEntry(entry, ""))
			}
			return sdk.TextResult(b.String()), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "cron_remove",
		Description: "Remove a scheduled cron task by name",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Task name to remove",
				},
			},
			"required": []string{"name"},
		},
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			name, _ := args["name"].(string)
			if name == "" {
				return sdk.ErrorResult("Task name required."), nil
			}

			cfg := LoadConfig()
			if _, ok := cfg.Tasks[name]; !ok {
				return sdk.ErrorResult(fmt.Sprintf("Task %q not found.", name)), nil
			}

			delete(cfg.Tasks, name)
			if err := SaveConfig(cfg); err != nil {
				return sdk.ErrorResult("Error saving: " + err.Error()), nil
			}
			return sdk.TextResult(fmt.Sprintf("Task %q removed.", name)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "cron_add",
		Description: "Add a new scheduled cron task",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Unique task name",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action type: shell, prompt, or webhook",
					"enum":        []string{"shell", "prompt", "webhook"},
				},
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command (for action=shell)",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Piglet prompt text (for action=prompt)",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "Webhook URL (for action=webhook)",
				},
				"every": map[string]any{
					"type":        "string",
					"description": "Interval schedule, e.g. '10m', '1h'",
				},
				"daily_at": map[string]any{
					"type":        "string",
					"description": "Daily schedule, e.g. '18:00'",
				},
				"weekly": map[string]any{
					"type":        "string",
					"description": "Weekly schedule, e.g. 'monday 09:00'",
				},
			},
			"required": []string{"name", "action"},
		},
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			name, _ := args["name"].(string)
			action, _ := args["action"].(string)

			task := TaskConfig{
				Action:  action,
				Command: argStr(args, "command"),
				Prompt:  argStr(args, "prompt"),
				URL:     argStr(args, "url"),
			}

			// Build and validate schedule spec.
			spec := ScheduleSpec{
				Every:   argStr(args, "every"),
				DailyAt: argStr(args, "daily_at"),
				Weekly:  argStr(args, "weekly"),
			}
			if _, err := ParseSchedule(spec); err != nil {
				return sdk.ErrorResult("Invalid schedule: " + err.Error()), nil
			}
			task.Schedule = spec

			cfg := LoadConfig()
			if cfg.Tasks == nil {
				cfg.Tasks = make(map[string]TaskConfig)
			}
			cfg.Tasks[name] = task

			if err := SaveConfig(cfg); err != nil {
				return sdk.ErrorResult("Error saving: " + err.Error()), nil
			}
			return sdk.TextResult(fmt.Sprintf("Task %q added (%s, %s).", name, action, spec)), nil
		},
	})
}

func registerEventHandler(e *sdk.Extension) {
	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "cron-status",
		Priority: 100,
		Events:   []string{"EventAgentStart"},
		Handle: func(_ context.Context, _ string, _ json.RawMessage) *sdk.Action {
			summaries, err := ListTasks()
			if err != nil || len(summaries) == 0 {
				return nil
			}

			enabled := 0
			overdue := 0
			for _, s := range summaries {
				if s.Enabled {
					enabled++
				}
				if s.Overdue {
					overdue++
				}
			}

			status := fmt.Sprintf("%d tasks", enabled)
			if overdue > 0 {
				status = fmt.Sprintf("%d tasks, %d overdue", enabled, overdue)
			}
			return sdk.ActionSetStatus("cron", status)
		},
	})
}

// Helper functions.

func argStr(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func pigletCronBin() string {
	// Check ~/go/bin first, then PATH.
	home, _ := os.UserHomeDir()
	bin := filepath.Join(home, "go", "bin", "piglet-cron")
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	if p, err := exec.LookPath("piglet-cron"); err == nil {
		return p
	}
	return ""
}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.piglet.cron.plist")
}

//go:embed defaults/launchd.plist
var plistTmpl string

func generatePlist(binPath string) string {
	configDir, err := xdg.ConfigDir()
	if err != nil {
		// Fall back to a sensible default if config dir is unavailable.
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config", "piglet")
	}
	logDir := filepath.Join(configDir, "logs")
	os.MkdirAll(logDir, 0o755) //nolint:errcheck // best-effort log dir creation

	tmpl := template.Must(template.New("plist").Parse(plistTmpl))
	var b strings.Builder
	_ = tmpl.Execute(&b, map[string]string{
		"BinPath": binPath,
		"LogDir":  logDir,
	})
	return b.String()
}
