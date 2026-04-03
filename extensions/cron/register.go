package cron

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

// Register registers the cron extension's commands, tools, and event handlers.
func Register(e *sdk.Extension) {
	registerCommands(e)
	registerTools(e)
	registerEventHandler(e)
}

func registerCommands(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "cron",
		Description: "Manage scheduled tasks (list, test, history, add, remove, install, uninstall, status)",
		Handler: func(ctx context.Context, args string) error {
			sub := strings.TrimSpace(args)
			switch {
			case sub == "" || sub == "list":
				return handleCronList(e)
			case sub == "status":
				return handleCronStatus(e)
			case strings.HasPrefix(sub, "test "):
				return handleCronTest(e, strings.TrimPrefix(sub, "test "))
			case strings.HasPrefix(sub, "history"):
				name := strings.TrimSpace(strings.TrimPrefix(sub, "history"))
				return handleCronHistory(e, name)
			case strings.HasPrefix(sub, "add"):
				return handleCronAdd(e)
			case strings.HasPrefix(sub, "remove "):
				return handleCronRemove(e, strings.TrimPrefix(sub, "remove "))
			case sub == "install":
				return handleCronInstall(e)
			case sub == "uninstall":
				return handleCronUninstall(e)
			default:
				e.ShowMessage("Unknown subcommand: " + sub + "\nUsage: /cron [list|test|history|add|remove|install|uninstall|status]")
			}
			return nil
		},
	})
}

func handleCronList(e *sdk.Extension) error {
	summaries, err := ListTasks()
	if err != nil {
		e.ShowMessage("Error: " + err.Error())
		return nil
	}
	if len(summaries) == 0 {
		if schedDir, err := xdg.ExtensionDir("cron"); err == nil {
			e.ShowMessage("No tasks configured. Edit " + filepath.Join(schedDir, "schedules.yaml") + " to add tasks.")
		} else {
			e.ShowMessage("No tasks configured. Add tasks via the schedules.yaml config file.")
		}
		return nil
	}

	// Sort by name for deterministic output.
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})

	var b strings.Builder
	b.WriteString("**Scheduled Tasks**\n\n")
	for _, s := range summaries {
		status := "enabled"
		if !s.Enabled {
			status = "disabled"
		}
		if s.Overdue {
			status = "OVERDUE"
		}

		lastRun := "never"
		if !s.LastRun.IsZero() {
			lastRun = s.LastRun.Format("2006-01-02 15:04")
		}
		nextRun := s.NextRun.Format("2006-01-02 15:04")

		fmt.Fprintf(&b, "- **%s** [%s] — %s (%s)\n  Last: %s | Next: %s\n",
			s.Name, s.Action, s.Schedule, status, lastRun, nextRun)
	}
	e.ShowMessage(b.String())
	return nil
}

func handleCronStatus(e *sdk.Extension) error {
	// Check if launchd agent is loaded.
	cmd := exec.Command("launchctl", "list", "com.piglet.cron")
	out, err := cmd.CombinedOutput()

	var b strings.Builder
	if err != nil {
		b.WriteString("**Cron Status**: not installed\n")
		b.WriteString("Run `/cron install` to set up the launchd agent.\n")
	} else {
		b.WriteString("**Cron Status**: installed\n")
		b.WriteString("```\n")
		b.WriteString(strings.TrimSpace(string(out)))
		b.WriteString("\n```\n")
	}

	// Show task summary.
	summaries, _ := ListTasks()
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
	fmt.Fprintf(&b, "\nTasks: %d total, %d enabled", len(summaries), enabled)
	if overdue > 0 {
		fmt.Fprintf(&b, ", **%d overdue**", overdue)
	}
	b.WriteString("\n")

	e.ShowMessage(b.String())
	return nil
}

func handleCronTest(e *sdk.Extension, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		e.ShowMessage("Usage: /cron test <task-name>")
		return nil
	}

	// Delegate to standalone binary.
	bin := pigletCronBin()
	if bin == "" {
		e.ShowMessage("Error: piglet-cron binary not found. Run `make cli-piglet-cron` to build it.")
		return nil
	}

	e.ShowMessage(fmt.Sprintf("Running task **%s**...", name))

	cmd := exec.Command(bin, "run", "--verbose", "--task", name)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		e.ShowMessage(fmt.Sprintf("Task **%s** failed:\n```\n%s\n```", name, output))
	} else {
		e.ShowMessage(fmt.Sprintf("Task **%s** completed.\n```\n%s\n```", name, output))
	}
	return nil
}

func handleCronHistory(e *sdk.Extension, name string) error {
	entries, err := ReadHistory()
	if err != nil {
		e.ShowMessage("Error reading history: " + err.Error())
		return nil
	}

	if name != "" {
		var filtered []RunEntry
		for _, entry := range entries {
			if entry.Task == name {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}

	if len(entries) == 0 {
		e.ShowMessage("No history found.")
		return nil
	}

	// Show last 20 entries.
	start := 0
	if len(entries) > 20 {
		start = len(entries) - 20
	}

	var b strings.Builder
	b.WriteString("**Recent History**\n\n")
	for _, entry := range entries[start:] {
		status := "ok"
		if !entry.Success {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "- %s [%s] %s (%dms)",
			entry.RanAt, status, entry.Task, entry.DurationMs)
		if entry.Error != "" {
			fmt.Fprintf(&b, " — %s", entry.Error)
		}
		b.WriteString("\n")
	}
	e.ShowMessage(b.String())
	return nil
}

func handleCronAdd(e *sdk.Extension) error {
	if schedDir, err := xdg.ExtensionDir("cron"); err == nil {
		e.ShowMessage("Edit `" + filepath.Join(schedDir, "schedules.yaml") + "` to add tasks.\nSee the file for examples and documentation.")
	} else {
		e.ShowMessage("Add tasks via the schedules.yaml config file. See the file for examples and documentation.")
	}
	return nil
}

func handleCronRemove(e *sdk.Extension, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		e.ShowMessage("Usage: /cron remove <task-name>")
		return nil
	}

	cfg := LoadConfig()
	if _, ok := cfg.Tasks[name]; !ok {
		e.ShowMessage(fmt.Sprintf("Task **%s** not found.", name))
		return nil
	}

	delete(cfg.Tasks, name)
	if err := SaveConfig(cfg); err != nil {
		e.ShowMessage("Error saving config: " + err.Error())
		return nil
	}
	e.ShowMessage(fmt.Sprintf("Task **%s** removed.", name))
	return nil
}

func handleCronInstall(e *sdk.Extension) error {
	bin := pigletCronBin()
	if bin == "" {
		e.ShowMessage("Error: piglet-cron binary not found. Run `make cli-piglet-cron` to build it.")
		return nil
	}

	plist := generatePlist(bin)
	path := plistPath()

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.ShowMessage("Error creating LaunchAgents dir: " + err.Error())
		return nil
	}

	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		e.ShowMessage("Error writing plist: " + err.Error())
		return nil
	}

	// Load the agent.
	target := fmt.Sprintf("gui/%d", os.Getuid())
	exec.Command("launchctl", "bootout", target, path).Run() //nolint:errcheck // may not be loaded yet

	cmd := exec.Command("launchctl", "bootstrap", target, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		e.ShowMessage(fmt.Sprintf("Error loading agent: %s\n%s", err, string(out)))
		return nil
	}

	e.ShowMessage(fmt.Sprintf("Cron agent installed and loaded.\nPlist: %s\nBinary: %s\nInterval: 60s", path, bin))
	return nil
}

func handleCronUninstall(e *sdk.Extension) error {
	path := plistPath()

	target := fmt.Sprintf("gui/%d", os.Getuid())
	exec.Command("launchctl", "bootout", target, path).Run() //nolint:errcheck // may not be loaded

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		e.ShowMessage("Error removing plist: " + err.Error())
		return nil
	}

	e.ShowMessage("Cron agent uninstalled.")
	return nil
}

func registerTools(e *sdk.Extension) {
	e.RegisterTool(sdk.ToolDef{
		Name:        "cron_list",
		Description: "List all scheduled cron tasks with their schedules, last run, and next run times",
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

			sort.Slice(summaries, func(i, j int) bool {
				return summaries[i].Name < summaries[j].Name
			})

			var b strings.Builder
			for _, s := range summaries {
				lastRun := "never"
				if !s.LastRun.IsZero() {
					lastRun = s.LastRun.Format(time.RFC3339)
				}
				status := "enabled"
				if !s.Enabled {
					status = "disabled"
				}
				if s.Overdue {
					status = "OVERDUE"
				}
				fmt.Fprintf(&b, "%s: action=%s schedule=%q status=%s last_run=%s next_run=%s\n",
					s.Name, s.Action, s.Schedule, status, lastRun, s.NextRun.Format(time.RFC3339))
			}
			return sdk.TextResult(b.String()), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "cron_history",
		Description: "Show recent cron task execution history",
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
				status := "ok"
				if !entry.Success {
					status = "FAIL"
				}
				fmt.Fprintf(&b, "%s [%s] %s (%dms)", entry.RanAt, status, entry.Task, entry.DurationMs)
				if entry.Error != "" {
					fmt.Fprintf(&b, " — %s", entry.Error)
				}
				b.WriteString("\n")
			}
			return sdk.TextResult(b.String()), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "cron_remove",
		Description: "Remove a scheduled cron task by name",
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
