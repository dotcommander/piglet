package cron

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// Register registers the cron extension's commands, tools, and event handlers.
func Register(e *sdk.Extension) {
	registerCommands(e)
	registerTools(e)
	registerEventHandler(e)
}

// countTaskStatus returns the number of enabled and overdue tasks.
func countTaskStatus(summaries []TaskSummary) (enabled, overdue int) {
	for _, s := range summaries {
		if s.Enabled {
			enabled++
		}
		if s.Overdue {
			overdue++
		}
	}
	return enabled, overdue
}

// formatHistoryEntry formats a single history entry with an optional prefix.
func formatHistoryEntry(entry RunEntry, prefix string) string {
	status := "ok"
	if !entry.Success {
		status = "FAIL"
	}
	var b strings.Builder
	if prefix != "" {
		b.WriteString(prefix)
	}
	fmt.Fprintf(&b, "%s [%s] %s (%dms)", entry.RanAt, status, entry.Task, entry.DurationMs)
	if entry.Error != "" {
		fmt.Fprintf(&b, " — %s", entry.Error)
	}
	b.WriteString("\n")
	return b.String()
}

// formatTaskList sorts tasks by name and formats them. When markdown is true,
// uses bold formatting suitable for TUI display; otherwise uses plain key=value
// format suitable for tool results.
func formatTaskList(tasks []TaskSummary, markdown bool) string {
	slices.SortFunc(tasks, func(a, b TaskSummary) int {
		return cmp.Compare(a.Name, b.Name)
	})

	var b strings.Builder
	if markdown {
		b.WriteString("**Scheduled Tasks**\n\n")
	}
	for _, s := range tasks {
		status := "enabled"
		if !s.Enabled {
			status = "disabled"
		}
		if s.Overdue {
			status = "OVERDUE"
		}

		lastRun := "never"
		if !s.LastRun.IsZero() {
			if markdown {
				lastRun = s.LastRun.Format("2006-01-02 15:04")
			} else {
				lastRun = s.LastRun.Format(time.RFC3339)
			}
		}
		nextRun := s.NextRun.Format(time.RFC3339)

		if markdown {
			fmt.Fprintf(&b, "- **%s** [%s] — %s (%s)\n  Last: %s | Next: %s\n",
				s.Name, s.Action, s.Schedule, status, lastRun, nextRun)
		} else {
			fmt.Fprintf(&b, "%s: action=%s schedule=%q status=%s last_run=%s next_run=%s\n",
				s.Name, s.Action, s.Schedule, status, lastRun, nextRun)
		}
	}
	return b.String()
}
