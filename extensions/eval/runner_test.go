package eval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestComputeSummary(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		s := computeSummary(nil)
		assert.Equal(t, 0, s.Total)
		assert.Equal(t, 0, s.Passed)
		assert.Equal(t, 0, s.Failed)
		assert.Equal(t, 0.0, s.AvgScore)
	})

	t.Run("all pass", func(t *testing.T) {
		t.Parallel()
		cases := []CaseResult{
			{Score: 1.0, Pass: true, DurationMs: 10},
			{Score: 1.0, Pass: true, DurationMs: 20},
		}
		s := computeSummary(cases)
		assert.Equal(t, 2, s.Total)
		assert.Equal(t, 2, s.Passed)
		assert.Equal(t, 0, s.Failed)
		assert.InDelta(t, 1.0, s.AvgScore, 0.001)
		assert.Equal(t, int64(30), s.TotalDurationMs)
	})

	t.Run("mixed", func(t *testing.T) {
		t.Parallel()
		cases := []CaseResult{
			{Score: 1.0, Pass: true, DurationMs: 100},
			{Score: 0.5, Pass: true, DurationMs: 200},
			{Score: 0.0, Pass: false, DurationMs: 50},
		}
		s := computeSummary(cases)
		assert.Equal(t, 3, s.Total)
		assert.Equal(t, 2, s.Passed)
		assert.Equal(t, 1, s.Failed)
		assert.InDelta(t, 0.5, s.AvgScore, 0.001)
		assert.Equal(t, int64(350), s.TotalDurationMs)
	})

	t.Run("run result round-trip", func(t *testing.T) {
		t.Parallel()
		cases := []CaseResult{
			{Name: "a", Score: 0.8, Pass: true, DurationMs: 15},
			{Name: "b", Score: 0.2, Pass: false, DurationMs: 25},
		}
		result := makeResult("my-suite", time.Now(), cases)
		assert.Equal(t, 2, result.Summary.Total)
		assert.Equal(t, 1, result.Summary.Passed)
		assert.InDelta(t, 0.5, result.Summary.AvgScore, 0.001)
		assert.Equal(t, int64(40), result.Summary.TotalDurationMs)
	})
}
