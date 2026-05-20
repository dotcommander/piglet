package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Retries ───────────────────────────────────────────────────────────────────

func TestRetryOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	counter := filepath.Join(dir, "count.txt")

	// Script: increment a counter file; succeed on attempt >= 2
	script := fmt.Sprintf(`
count=$(cat %s 2>/dev/null || echo 0)
count=$((count + 1))
echo $count > %s
if [ $count -lt 2 ]; then
  exit 1
fi
echo "succeeded on attempt $count"
`, counter, counter)

	p := &Pipeline{
		Name: "retry-pipe",
		Steps: []Step{
			{
				Name:    "flaky",
				Run:     script,
				Retries: 2,
				// RetryDelay intentionally omitted: defaults() sets it to 5s
				// but step succeeds on attempt 2, so only one 5s wait occurs.
				Timeout: 10,
			},
		},
	}

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)

	sr := result.Steps[0]
	assert.Equal(t, "ok", sr.Status)
	assert.GreaterOrEqual(t, sr.RetryCount, 1)
	assert.Contains(t, sr.Output, "succeeded")
}

func TestRetryExhausted(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "exhaust-pipe",
		Steps: []Step{
			{
				Name:    "always-fails",
				Run:     "exit 1",
				Retries: 2,
				// RetryDelay omitted: defaults() sets 5s; 2 retries = ~10s total.
				Timeout: 30,
			},
		},
	}

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)

	sr := result.Steps[0]
	assert.Equal(t, "error", sr.Status)
	assert.Equal(t, 2, sr.RetryCount)
}

// ── AllowFailure ──────────────────────────────────────────────────────────────

func TestAllowFailure(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "allow-fail-pipe",
		Steps: []Step{
			{Name: "may-fail", Run: "exit 1", AllowFailure: true},
			{Name: "continues", Run: "echo after-failure"},
		},
	}

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err)

	// Pipeline continues but status is "partial"
	assert.Equal(t, "partial", result.Status)
	require.Len(t, result.Steps, 2)
	assert.Equal(t, "error", result.Steps[0].Status)
	assert.Equal(t, "ok", result.Steps[1].Status)
	assert.Equal(t, "after-failure", result.Steps[1].Output)
}

// ── When predicate ────────────────────────────────────────────────────────────

func TestWhenPredicate(t *testing.T) {
	t.Parallel()

	t.Run("predicate passes — step runs", func(t *testing.T) {
		t.Parallel()
		p := &Pipeline{
			Name: "when-pass",
			Steps: []Step{
				{Name: "conditional", Run: "echo ran", When: "true"},
			},
		}
		result, err := Run(context.Background(), p, nil)
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Status)
		assert.Equal(t, "ok", result.Steps[0].Status)
		assert.Equal(t, "ran", result.Steps[0].Output)
	})

	t.Run("predicate fails — step skipped", func(t *testing.T) {
		t.Parallel()
		p := &Pipeline{
			Name: "when-fail",
			Steps: []Step{
				{Name: "conditional", Run: "echo ran", When: "false"},
				{Name: "after", Run: "echo after"},
			},
		}
		result, err := Run(context.Background(), p, nil)
		require.NoError(t, err)
		// overall pipeline still ok (skipped != error)
		assert.Equal(t, "ok", result.Status)
		require.Len(t, result.Steps, 2)
		assert.Equal(t, "skipped", result.Steps[0].Status)
		assert.Contains(t, result.Steps[0].Output, "when predicate failed")
		assert.Equal(t, "ok", result.Steps[1].Status)
	})

	t.Run("predicate uses param", func(t *testing.T) {
		t.Parallel()
		p := &Pipeline{
			Name: "when-param",
			Params: map[string]Param{
				"skip": {Default: "false"},
			},
			Steps: []Step{
				{
					Name: "guarded",
					Run:  "echo guarded-ran",
					When: `[ "{param.skip}" = "false" ]`,
				},
			},
		}
		// param.skip = "false" → predicate [ "false" = "false" ] → true → step runs
		result, err := Run(context.Background(), p, map[string]string{"skip": "false"})
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Steps[0].Status)

		// param.skip = "true" → predicate [ "true" = "false" ] → false → step skipped
		result2, err := Run(context.Background(), p, map[string]string{"skip": "true"})
		require.NoError(t, err)
		assert.Equal(t, "skipped", result2.Steps[0].Status)
	})
}
