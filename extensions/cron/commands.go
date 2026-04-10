package cron

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

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

	e.ShowMessage(formatTaskList(summaries, true))
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
	enabled, overdue := countTaskStatus(summaries)
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

	filtered := filterHistory(entries, name, 20)
	if len(filtered) == 0 {
		e.ShowMessage("No history found.")
		return nil
	}

	var b strings.Builder
	b.WriteString("**Recent History**\n\n")
	for _, entry := range filtered {
		b.WriteString(formatHistoryEntry(entry, "- "))
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
