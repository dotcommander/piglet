package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeResult(suite string, ranAt time.Time, cases []CaseResult) *RunResult {
	r := &RunResult{
		Suite: suite,
		Model: "small",
		RanAt: ranAt,
		Cases: cases,
	}
	r.Summary = computeSummary(cases)
	return r
}

func writeResultToDir(t *testing.T, dir string, r *RunResult) string {
	t.Helper()
	data, err := json.Marshal(r)
	require.NoError(t, err)
	ts := r.RanAt.UTC().Format("20060102-150405")
	name := sanitizeName(r.Suite) + "-" + ts + ".json"
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// listResultsInDir is a test-local variant of ListResults that accepts an explicit dir.
func listResultsInDir(dir, suiteFilter string) ([]ResultSummary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var summaries []ResultSummary
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		r, err := LoadResult(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if suiteFilter != "" && r.Suite != suiteFilter {
			continue
		}
		summaries = append(summaries, ResultSummary{
			Path:   filepath.Join(dir, e.Name()),
			Suite:  r.Suite,
			RanAt:  r.RanAt,
			Passed: r.Summary.Passed,
			Total:  r.Summary.Total,
		})
	}
	return summaries, nil
}

func TestSaveAndLoadResult(t *testing.T) {
	t.Parallel()

	ranAt := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	result := makeResult("my-suite", ranAt, []CaseResult{
		{Name: "c1", Prompt: "hello", Response: "hi", Score: 1.0, Pass: true, Reason: "exact match", DurationMs: 42},
		{Name: "c2", Prompt: "world", Response: "nope", Score: 0.0, Pass: false, Reason: "mismatch", DurationMs: 17},
	})

	dir := t.TempDir()
	path := writeResultToDir(t, dir, result)

	loaded, err := LoadResult(path)
	require.NoError(t, err)

	assert.Equal(t, result.Suite, loaded.Suite)
	assert.Equal(t, result.Model, loaded.Model)
	assert.WithinDuration(t, result.RanAt, loaded.RanAt, time.Second)
	require.Len(t, loaded.Cases, 2)
	assert.Equal(t, "c1", loaded.Cases[0].Name)
	assert.Equal(t, 1.0, loaded.Cases[0].Score)
	assert.Equal(t, int64(42), loaded.Cases[0].DurationMs)
	assert.Equal(t, 1, loaded.Summary.Passed)
	assert.Equal(t, 1, loaded.Summary.Failed)
}

func TestListResults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	r1 := makeResult("suite-a", time.Now(), []CaseResult{
		{Name: "c1", Pass: true, Score: 1.0},
	})
	r2 := makeResult("suite-b", time.Now().Add(time.Second), []CaseResult{
		{Name: "c1", Pass: false, Score: 0.0},
		{Name: "c2", Pass: true, Score: 1.0},
	})

	writeResultToDir(t, dir, r1)
	writeResultToDir(t, dir, r2)

	summaries, err := listResultsInDir(dir, "")
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	byName := make(map[string]ResultSummary)
	for _, s := range summaries {
		byName[s.Suite] = s
	}

	assert.Equal(t, 1, byName["suite-a"].Passed)
	assert.Equal(t, 1, byName["suite-a"].Total)
	assert.Equal(t, 1, byName["suite-b"].Passed)
	assert.Equal(t, 2, byName["suite-b"].Total)
}

func TestCompare(t *testing.T) {
	t.Parallel()

	a := makeResult("suite", time.Now(), []CaseResult{
		{Name: "c1", Score: 0.5, Pass: false},
		{Name: "c2", Score: 1.0, Pass: true},
		{Name: "c3", Score: 0.0, Pass: false},
	})
	b := makeResult("suite", time.Now(), []CaseResult{
		{Name: "c1", Score: 1.0, Pass: true},
		{Name: "c2", Score: 0.5, Pass: true},
		// c3 missing from b — no delta generated
	})

	comp := Compare(a, b)
	require.Len(t, comp.Deltas, 2)

	byName := make(map[string]CaseDelta)
	for _, d := range comp.Deltas {
		byName[d.Name] = d
	}

	d1 := byName["c1"]
	assert.InDelta(t, 0.5, d1.Delta, 0.001)
	assert.False(t, d1.PassA)
	assert.True(t, d1.PassB)

	d2 := byName["c2"]
	assert.InDelta(t, -0.5, d2.Delta, 0.001)
}

func TestCompareFormat(t *testing.T) {
	t.Parallel()

	a := makeResult("suite", time.Now(), []CaseResult{
		{Name: "my-case", Score: 0.5, Pass: false},
	})
	b := makeResult("suite", time.Now(), []CaseResult{
		{Name: "my-case", Score: 1.0, Pass: true},
	})

	comp := Compare(a, b)
	out := comp.Format()

	assert.Contains(t, out, "my-case")
	assert.Contains(t, out, "ScoreA")
	assert.Contains(t, out, "ScoreB")
	assert.Contains(t, out, "Delta")
	assert.Contains(t, out, "PassA")
	assert.Contains(t, out, "PassB")
}
