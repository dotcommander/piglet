package cron

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- LastRun ----------

func TestLastRunZeroOnEmpty(t *testing.T) {
	t.Parallel()
	got := LastRun(nil, "anything")
	assert.True(t, got.IsZero())
}

func TestLastRunIgnoresFailedEntries(t *testing.T) {
	t.Parallel()
	entries := []RunEntry{
		{Task: "a", RanAt: "2026-04-01T09:00:00Z", Success: true},
		{Task: "a", RanAt: "2026-04-01T10:00:00Z", Success: false}, // most recent but failed
	}
	got := LastRun(entries, "a")
	want, _ := time.Parse(time.RFC3339, "2026-04-01T09:00:00Z")
	assert.Equal(t, want, got)
}

func TestLastRunOtherTask(t *testing.T) {
	t.Parallel()
	entries := []RunEntry{
		{Task: "other", RanAt: "2026-04-01T09:00:00Z", Success: true},
	}
	got := LastRun(entries, "missing")
	assert.True(t, got.IsZero())
}

func TestLastRunMalformedTimestamp(t *testing.T) {
	t.Parallel()
	// Most recent entry has a bad timestamp; helper should fall back to the
	// next successful entry with a parseable RanAt.
	entries := []RunEntry{
		{Task: "a", RanAt: "2026-04-01T09:00:00Z", Success: true},
		{Task: "a", RanAt: "not-a-time", Success: true},
	}
	want, _ := time.Parse(time.RFC3339, "2026-04-01T09:00:00Z")
	assert.Equal(t, want, LastRun(entries, "a"))
}

// ---------- filterHistory ----------

func TestFilterHistoryNoEntries(t *testing.T) {
	t.Parallel()
	assert.Nil(t, filterHistory(nil, "", 10))
}

func TestFilterHistoryByTaskName(t *testing.T) {
	t.Parallel()
	entries := []RunEntry{
		{Task: "a"}, {Task: "b"}, {Task: "a"}, {Task: "c"},
	}
	got := filterHistory(entries, "a", 10)
	require.Len(t, got, 2)
	for _, e := range got {
		assert.Equal(t, "a", e.Task)
	}
}

func TestFilterHistoryLimitTrailing(t *testing.T) {
	t.Parallel()
	entries := []RunEntry{
		{Task: "a", RanAt: "1"},
		{Task: "a", RanAt: "2"},
		{Task: "a", RanAt: "3"},
		{Task: "a", RanAt: "4"},
	}
	got := filterHistory(entries, "", 2)
	require.Len(t, got, 2)
	assert.Equal(t, "3", got[0].RanAt)
	assert.Equal(t, "4", got[1].RanAt)
}

func TestFilterHistoryLimitLargerThanLen(t *testing.T) {
	t.Parallel()
	entries := []RunEntry{{Task: "a"}, {Task: "a"}}
	got := filterHistory(entries, "", 100)
	assert.Equal(t, entries, got)
}

// ---------- TaskConfig / countTaskStatus ----------

func TestTaskConfigIsEnabledDefaults(t *testing.T) {
	t.Parallel()
	assert.True(t, TaskConfig{}.IsEnabled(), "nil pointer = enabled")

	tr, fl := true, false
	assert.True(t, TaskConfig{Enabled: &tr}.IsEnabled())
	assert.False(t, TaskConfig{Enabled: &fl}.IsEnabled())
}

func TestCountTaskStatus(t *testing.T) {
	t.Parallel()
	tasks := []TaskSummary{
		{Name: "a", Enabled: true},
		{Name: "b", Enabled: false},
		{Name: "c", Enabled: true, Overdue: true},
		{Name: "d", Enabled: false, Overdue: true},
	}
	enabled, overdue := countTaskStatus(tasks)
	assert.Equal(t, 2, enabled)
	assert.Equal(t, 2, overdue)
}

// ---------- small string/arg helpers ----------

func TestArgStrReturnsEmptyForMissingOrWrongType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", argStr(nil, "x"))
	assert.Equal(t, "", argStr(map[string]any{"x": 123}, "x"))
	assert.Equal(t, "hello", argStr(map[string]any{"x": "hello"}, "x"))
}

func TestTruncateOutput(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "abc", truncateOutput("abc", 10))
	assert.Equal(t, "abc", truncateOutput("abc", 3))

	long := strings.Repeat("x", 20)
	got := truncateOutput(long, 5)
	assert.Equal(t, "xxxxx...(truncated)", got)
}

func TestMapToEnvDeterministic(t *testing.T) {
	t.Parallel()
	m := map[string]string{"A": "1", "B": "2", "C": "3"}
	got := mapToEnv(m)

	// Order from map iteration is unspecified; sort before comparing.
	sort.Strings(got)
	assert.Equal(t, []string{"A=1", "B=2", "C=3"}, got)
}

// ---------- ParseSchedule edge cases (complement TestParseSchedule) ----------

func TestParseScheduleWeeklyMissingTime(t *testing.T) {
	t.Parallel()
	_, err := ParseSchedule(ScheduleSpec{Weekly: "monday"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "weekday HH:MM")
}

func TestParseScheduleWeeklyBadTime(t *testing.T) {
	t.Parallel()
	_, err := ParseSchedule(ScheduleSpec{Weekly: "monday 25:00"})
	require.Error(t, err)
}
