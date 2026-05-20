package cron

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withIsolatedConfig points XDG_CONFIG_HOME at a t.TempDir so history/config
// helpers operate on a per-test directory and never touch the real
// ~/.config/piglet tree. Restores any prior value via t.Cleanup.
//
// Tests using this helper MUST NOT call t.Parallel(): XDG_CONFIG_HOME is a
// process-wide env var and concurrent tests would race.
func withIsolatedConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev, hadPrev := os.LookupEnv("XDG_CONFIG_HOME")
	require.NoError(t, os.Setenv("XDG_CONFIG_HOME", dir))
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("XDG_CONFIG_HOME", prev)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
	})
	return dir
}

func TestHistoryPath(t *testing.T) {
	dir := withIsolatedConfig(t)

	p, err := historyPath()
	require.NoError(t, err)

	want := filepath.Join(dir, "piglet", "extensions", "cron", historyFile)
	assert.Equal(t, want, p)
}

func TestAppendHistoryCreatesFileAndDir(t *testing.T) {
	withIsolatedConfig(t)

	entry := RunEntry{
		Task:       "demo",
		RanAt:      time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Success:    true,
		DurationMs: 42,
	}
	require.NoError(t, AppendHistory(entry))

	p, err := historyPath()
	require.NoError(t, err)
	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	// History file should be user-only readable/writable.
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestAppendHistoryRoundTrip(t *testing.T) {
	withIsolatedConfig(t)

	want := []RunEntry{
		{Task: "alpha", RanAt: "2026-04-01T09:00:00Z", Success: true, DurationMs: 10},
		{Task: "beta", RanAt: "2026-04-01T09:05:00Z", Success: false, DurationMs: 25, Error: "boom"},
		{Task: "alpha", RanAt: "2026-04-01T09:10:00Z", Success: true, DurationMs: 8},
	}
	for _, e := range want {
		require.NoError(t, AppendHistory(e))
	}

	got, err := ReadHistory()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestReadHistoryMissingFileReturnsNil(t *testing.T) {
	withIsolatedConfig(t)

	entries, err := ReadHistory()
	require.NoError(t, err)
	assert.Nil(t, entries)
}

func TestReadHistorySkipsMalformedLines(t *testing.T) {
	withIsolatedConfig(t)

	// Append a good entry, then a malformed line, then another good entry.
	good1 := RunEntry{Task: "a", RanAt: "2026-04-01T09:00:00Z", Success: true}
	good2 := RunEntry{Task: "b", RanAt: "2026-04-01T09:01:00Z", Success: true}
	require.NoError(t, AppendHistory(good1))

	// Inject a malformed line directly between the two valid entries.
	p, err := historyPath()
	require.NoError(t, err)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("not-json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.NoError(t, AppendHistory(good2))

	entries, err := ReadHistory()
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "a", entries[0].Task)
	assert.Equal(t, "b", entries[1].Task)
}

func TestRotateHistoryNoOpBelowThreshold(t *testing.T) {
	withIsolatedConfig(t)

	// Two entries; well below maxHistoryLines.
	require.NoError(t, AppendHistory(RunEntry{Task: "a", RanAt: "2026-04-01T09:00:00Z"}))
	require.NoError(t, AppendHistory(RunEntry{Task: "b", RanAt: "2026-04-01T09:01:00Z"}))

	require.NoError(t, RotateHistory())

	entries, err := ReadHistory()
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestRotateHistoryTruncatesAboveThreshold(t *testing.T) {
	withIsolatedConfig(t)

	// Write maxHistoryLines+5 entries; expect rotate to keep only the most
	// recent maxHistoryLines and preserve order.
	extra := 5
	for i := 0; i < maxHistoryLines+extra; i++ {
		require.NoError(t, AppendHistory(RunEntry{
			Task:  "t",
			RanAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC).Format(time.RFC3339),
		}))
	}

	require.NoError(t, RotateHistory())

	entries, err := ReadHistory()
	require.NoError(t, err)
	require.Len(t, entries, maxHistoryLines)

	// First retained entry should correspond to the (extra)-th original entry.
	wantFirst := time.Date(2026, 1, 1, 0, 0, extra, 0, time.UTC).Format(time.RFC3339)
	assert.Equal(t, wantFirst, entries[0].RanAt)

	// Trailing entry is the latest one written.
	wantLast := time.Date(2026, 1, 1, 0, 0, maxHistoryLines+extra-1, 0, time.UTC).Format(time.RFC3339)
	assert.Equal(t, wantLast, entries[len(entries)-1].RanAt)

	// Rotated file should retain 0o600 permissions.
	p, err := historyPath()
	require.NoError(t, err)
	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestRotateHistoryOnEmptyHistoryIsNoOp(t *testing.T) {
	withIsolatedConfig(t)

	// No history file at all — rotate should not error.
	require.NoError(t, RotateHistory())

	entries, err := ReadHistory()
	require.NoError(t, err)
	assert.Nil(t, entries)
}

func TestAppendHistoryWritesValidJSONL(t *testing.T) {
	withIsolatedConfig(t)

	entry := RunEntry{
		Task:       "demo",
		RanAt:      "2026-04-01T09:00:00Z",
		Success:    true,
		DurationMs: 7,
	}
	require.NoError(t, AppendHistory(entry))

	p, err := historyPath()
	require.NoError(t, err)
	raw, err := os.ReadFile(p) //nolint:gosec // G304: path is inside t.TempDir
	require.NoError(t, err)

	// File ends in a newline so subsequent appends start on a fresh line.
	assert.True(t, strings.HasSuffix(string(raw), "\n"), "history line must end with newline")

	var decoded RunEntry
	require.NoError(t, json.Unmarshal([]byte(strings.TrimRight(string(raw), "\n")), &decoded))
	assert.Equal(t, entry, decoded)
}
