package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── ExpandIterations ──────────────────────────────────────────────────────────

func TestExpandIterations(t *testing.T) {
	t.Parallel()

	t.Run("no loops returns nil", func(t *testing.T) {
		t.Parallel()
		step := &Step{Name: "s", Run: "echo"}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		assert.Nil(t, iters)
	})

	t.Run("each only", func(t *testing.T) {
		t.Parallel()
		step := &Step{
			Name: "s",
			Run:  "echo {item}",
			Each: []string{"a", "b", "c"},
		}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		require.Len(t, iters, 3)
		assert.Equal(t, "a", iters[0].Item)
		assert.Equal(t, "b", iters[1].Item)
		assert.Equal(t, "c", iters[2].Item)
		for _, it := range iters {
			assert.Nil(t, it.LoopVars)
		}
	})

	t.Run("loop numeric range only", func(t *testing.T) {
		t.Parallel()
		step := &Step{
			Name: "s",
			Run:  "echo {loop.n}",
			Loop: map[string]any{"n": "1..4"},
		}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		require.Len(t, iters, 4)
		for _, it := range iters {
			assert.Empty(t, it.Item)
			assert.Contains(t, it.LoopVars, "n")
		}
		vals := make([]string, len(iters))
		for i, it := range iters {
			vals[i] = it.LoopVars["n"]
		}
		assert.Equal(t, []string{"1", "2", "3", "4"}, vals)
	})

	t.Run("both each and loop — cartesian product", func(t *testing.T) {
		t.Parallel()
		step := &Step{
			Name: "s",
			Run:  "echo {item}/{loop.n}",
			Each: []string{"x", "y"},
			Loop: map[string]any{"n": "1..2"},
		}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		// 2 items × 2 loop values = 4
		assert.Len(t, iters, 4)
	})

	t.Run("explicit list loop", func(t *testing.T) {
		t.Parallel()
		step := &Step{
			Name: "s",
			Run:  "echo {loop.env}",
			Loop: map[string]any{"env": []any{"dev", "staging", "prod"}},
		}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		require.Len(t, iters, 3)
		vals := []string{
			iters[0].LoopVars["env"],
			iters[1].LoopVars["env"],
			iters[2].LoopVars["env"],
		}
		assert.Equal(t, []string{"dev", "staging", "prod"}, vals)
	})
}

// ── ParseRange ────────────────────────────────────────────────────────────────

func TestParseRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "ascending 1..5",
			input: "1..5",
			want:  []string{"1", "2", "3", "4", "5"},
		},
		{
			name:  "negative range -3..3",
			input: "-3..3",
			want:  []string{"-3", "-2", "-1", "0", "1", "2", "3"},
		},
		{
			name:  "reverse 5..1",
			input: "5..1",
			want:  []string{"5", "4", "3", "2", "1"},
		},
		{
			name:  "single value no dots",
			input: "hello",
			want:  []string{"hello"},
		},
		{
			name:  "zero range 0..0",
			input: "0..0",
			want:  []string{"0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseRange(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ── ExpandDayRange ────────────────────────────────────────────────────────────

func TestExpandDayRange(t *testing.T) {
	t.Parallel()

	t.Run("7 days back to yesterday", func(t *testing.T) {
		t.Parallel()
		got := expandDayRange(-7, -1, time.Time{})
		assert.Len(t, got, 7)
		// all values should be valid dates
		for _, d := range got {
			_, err := time.Parse("2006-01-02", d)
			assert.NoError(t, err, "expected YYYY-MM-DD, got %q", d)
		}
		// first date should be 7 days ago, last should be yesterday
		now := time.Now()
		assert.Equal(t, now.AddDate(0, 0, -7).Format("2006-01-02"), got[0])
		assert.Equal(t, now.AddDate(0, 0, -1).Format("2006-01-02"), got[6])
	})

	t.Run("single day", func(t *testing.T) {
		t.Parallel()
		got := expandDayRange(0, 0, time.Time{})
		assert.Len(t, got, 1)
		assert.Equal(t, time.Now().Format("2006-01-02"), got[0])
	})

	t.Run("swapped start end normalised", func(t *testing.T) {
		t.Parallel()
		// expandDayRange swaps if startDays > endDays
		got := expandDayRange(-1, -7, time.Time{})
		assert.Len(t, got, 7)
	})
}

func TestParseRangeDayRange(t *testing.T) {
	t.Parallel()

	got, err := parseRange("-7d..-1d")
	require.NoError(t, err)
	assert.Len(t, got, 7)
	for _, d := range got {
		_, parseErr := time.Parse("2006-01-02", d)
		assert.NoError(t, parseErr)
	}
}

// ── CartesianLoop ─────────────────────────────────────────────────────────────

func TestCartesianLoop(t *testing.T) {
	t.Parallel()

	t.Run("empty dims returns single empty map", func(t *testing.T) {
		t.Parallel()
		got := cartesianLoop(nil)
		require.Len(t, got, 1)
		assert.Empty(t, got[0])
	})

	t.Run("2 dimensions", func(t *testing.T) {
		t.Parallel()
		dims := []loopDimension{
			{key: "color", values: []string{"red", "blue"}},
			{key: "size", values: []string{"S", "M", "L"}},
		}
		got := cartesianLoop(dims)
		// 2 × 3 = 6 combinations
		assert.Len(t, got, 6)

		// Every combination must have both keys
		for _, combo := range got {
			assert.Contains(t, combo, "color")
			assert.Contains(t, combo, "size")
		}

		// Verify all color×size pairs are represented
		seen := make(map[string]bool)
		for _, combo := range got {
			key := fmt.Sprintf("%s/%s", combo["color"], combo["size"])
			seen[key] = true
		}
		assert.True(t, seen["red/S"])
		assert.True(t, seen["red/M"])
		assert.True(t, seen["red/L"])
		assert.True(t, seen["blue/S"])
		assert.True(t, seen["blue/M"])
		assert.True(t, seen["blue/L"])
	})

	t.Run("single dimension", func(t *testing.T) {
		t.Parallel()
		dims := []loopDimension{
			{key: "n", values: []string{"1", "2"}},
		}
		got := cartesianLoop(dims)
		assert.Len(t, got, 2)
	})
}
