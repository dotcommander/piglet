package cron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setNow installs a fake clock for the duration of a test.
// Tests that touch nowFunc must NOT call t.Parallel() — nowFunc is shared state.
func setNow(t *testing.T, now time.Time) {
	t.Helper()
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = time.Now })
}

func TestIntervalSchedule(t *testing.T) {
	fixedNow := time.Date(2026, 3, 31, 12, 0, 0, 0, time.Local)
	s := IntervalSchedule{Interval: 10 * time.Minute}

	t.Run("first run", func(t *testing.T) {
		setNow(t, fixedNow)
		assert.True(t, s.ShouldRun(time.Time{}))
	})

	t.Run("not yet due", func(t *testing.T) {
		setNow(t, fixedNow)
		assert.False(t, s.ShouldRun(fixedNow.Add(-5*time.Minute)))
	})

	t.Run("due", func(t *testing.T) {
		setNow(t, fixedNow)
		assert.True(t, s.ShouldRun(fixedNow.Add(-10*time.Minute)))
	})

	t.Run("overdue", func(t *testing.T) {
		setNow(t, fixedNow)
		assert.True(t, s.ShouldRun(fixedNow.Add(-1*time.Hour)))
	})
}

func TestDailySchedule(t *testing.T) {
	s := DailySchedule{Hour: 18, Minute: 0}

	t.Run("first run before target", func(t *testing.T) {
		setNow(t, time.Date(2026, 3, 31, 10, 0, 0, 0, time.Local))
		assert.False(t, s.ShouldRun(time.Time{}))
	})

	t.Run("first run after target", func(t *testing.T) {
		setNow(t, time.Date(2026, 3, 31, 19, 0, 0, 0, time.Local))
		assert.True(t, s.ShouldRun(time.Time{}))
	})

	t.Run("already ran today", func(t *testing.T) {
		setNow(t, time.Date(2026, 3, 31, 20, 0, 0, 0, time.Local))
		lastRun := time.Date(2026, 3, 31, 18, 1, 0, 0, time.Local)
		assert.False(t, s.ShouldRun(lastRun))
	})

	t.Run("missed run", func(t *testing.T) {
		setNow(t, time.Date(2026, 4, 2, 10, 0, 0, 0, time.Local))
		lastRun := time.Date(2026, 3, 30, 18, 0, 0, 0, time.Local)
		assert.True(t, s.ShouldRun(lastRun))
	})
}

func TestWeeklySchedule(t *testing.T) {
	// Monday 09:00
	s := WeeklySchedule{Weekday: time.Monday, Hour: 9, Minute: 0}

	t.Run("first run on wrong day", func(t *testing.T) {
		// Wednesday 2026-04-01
		setNow(t, time.Date(2026, 4, 1, 10, 0, 0, 0, time.Local))
		assert.False(t, s.ShouldRun(time.Time{}))
	})

	t.Run("first run on correct day after time", func(t *testing.T) {
		// Monday 2026-03-30 at 10:00 (past 09:00 target)
		setNow(t, time.Date(2026, 3, 30, 10, 0, 0, 0, time.Local))
		assert.True(t, s.ShouldRun(time.Time{}))
	})

	t.Run("missed run over a week", func(t *testing.T) {
		setNow(t, time.Date(2026, 4, 8, 10, 0, 0, 0, time.Local))
		lastRun := time.Date(2026, 3, 23, 9, 0, 0, 0, time.Local)
		assert.True(t, s.ShouldRun(lastRun))
	})
}

func TestParseSchedule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    ScheduleSpec
		wantStr string
		wantErr bool
	}{
		{"interval", ScheduleSpec{Every: "10m"}, "every 10m0s", false},
		{"daily", ScheduleSpec{DailyAt: "18:00"}, "daily at 18:00", false},
		{"weekly", ScheduleSpec{Weekly: "monday 09:00"}, "weekly on Monday at 09:00", false},
		{"bad interval", ScheduleSpec{Every: "bad"}, "", true},
		{"too short", ScheduleSpec{Every: "30s"}, "", true},
		{"bad daily", ScheduleSpec{DailyAt: "25:00"}, "", true},
		{"bad weekly day", ScheduleSpec{Weekly: "notaday 09:00"}, "", true},
		{"empty", ScheduleSpec{}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := ParseSchedule(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStr, s.String())
		})
	}
}
