package cron

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- formatHistoryEntry ----------

func TestFormatHistoryEntrySuccess(t *testing.T) {
	t.Parallel()
	out := formatHistoryEntry(RunEntry{
		Task:       "demo",
		RanAt:      "2026-04-01T09:00:00Z",
		Success:    true,
		DurationMs: 12,
	}, "- ")
	assert.Equal(t, "- 2026-04-01T09:00:00Z [ok] demo (12ms)\n", out)
}

func TestFormatHistoryEntryFailureIncludesError(t *testing.T) {
	t.Parallel()
	out := formatHistoryEntry(RunEntry{
		Task:       "demo",
		RanAt:      "2026-04-01T09:00:00Z",
		Success:    false,
		DurationMs: 7,
		Error:      "boom",
	}, "")
	assert.Contains(t, out, "[FAIL]")
	assert.Contains(t, out, "— boom")
	assert.True(t, strings.HasSuffix(out, "\n"))
}

// ---------- formatTaskList ----------

func TestFormatTaskListSortsByName(t *testing.T) {
	t.Parallel()
	tasks := []TaskSummary{
		{Name: "zeta", Action: "shell", Schedule: "every 1h", Enabled: true, NextRun: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)},
		{Name: "alpha", Action: "prompt", Schedule: "daily at 09:00", Enabled: true, NextRun: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)},
	}
	plain := formatTaskList(tasks, false)
	alphaAt := strings.Index(plain, "alpha")
	zetaAt := strings.Index(plain, "zeta")
	require.NotEqual(t, -1, alphaAt)
	require.NotEqual(t, -1, zetaAt)
	assert.Less(t, alphaAt, zetaAt)
}

func TestFormatTaskListMarkdownVsPlain(t *testing.T) {
	t.Parallel()
	tasks := []TaskSummary{
		{Name: "t", Action: "shell", Schedule: "every 1h", Enabled: false},
	}

	md := formatTaskList(tasks, true)
	assert.Contains(t, md, "**Scheduled Tasks**")
	assert.Contains(t, md, "**t**")
	assert.Contains(t, md, "disabled")

	plain := formatTaskList(tasks, false)
	assert.NotContains(t, plain, "**")
	assert.Contains(t, plain, "t:")
	assert.Contains(t, plain, "status=disabled")
}

func TestFormatTaskListOverdueOverridesStatus(t *testing.T) {
	t.Parallel()
	tasks := []TaskSummary{
		{Name: "t", Enabled: true, Overdue: true},
	}
	plain := formatTaskList(tasks, false)
	assert.Contains(t, plain, "status=OVERDUE")
}

func TestFormatTaskListLastRunNever(t *testing.T) {
	t.Parallel()
	tasks := []TaskSummary{
		{Name: "t", Enabled: true}, // LastRun zero value
	}
	plain := formatTaskList(tasks, false)
	assert.Contains(t, plain, "last_run=never")
}
