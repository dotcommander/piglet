package toolbreaker

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracker_AccumulatesOnErrors(t *testing.T) {
	t.Parallel()

	tr := New()
	assert.Equal(t, 1, tr.RecordError("bash"))
	assert.Equal(t, 2, tr.RecordError("bash"))
	assert.Equal(t, 3, tr.RecordError("bash"))
	assert.Equal(t, 3, tr.Count("bash"))
}

func TestTracker_ResetsOnSuccess(t *testing.T) {
	t.Parallel()

	tr := New()
	tr.RecordError("bash")
	tr.RecordError("bash")
	tr.RecordSuccess("bash")
	assert.Equal(t, 0, tr.Count("bash"), "success must zero the counter")
}

func TestTracker_IsDisabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		errors int
		limit  int
		want   bool
	}{
		{"not yet disabled", 2, 3, false},
		{"exactly at limit", 3, 3, true},
		{"over limit", 5, 3, true},
		{"limit zero disables feature", 99, 0, false},
		{"limit one trip on first error", 1, 1, true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := New()
			for range tc.errors {
				tr.RecordError("tool")
			}
			assert.Equal(t, tc.want, tr.IsDisabled("tool", tc.limit))
		})
	}
}

func TestTracker_IndependentPerTool(t *testing.T) {
	t.Parallel()

	tr := New()
	tr.RecordError("bash")
	tr.RecordError("bash")
	tr.RecordError("bash")

	tr.RecordError("read")

	assert.True(t, tr.IsDisabled("bash", 3))
	assert.False(t, tr.IsDisabled("read", 3))
}

func TestTracker_Reset(t *testing.T) {
	t.Parallel()

	tr := New()
	tr.RecordError("bash")
	tr.RecordError("bash")
	tr.RecordError("bash")
	assert.True(t, tr.IsDisabled("bash", 3))

	tr.Reset()
	assert.False(t, tr.IsDisabled("bash", 3))
	assert.Equal(t, 0, tr.Count("bash"))
}

func TestTracker_ConcurrentRecordError(t *testing.T) {
	t.Parallel()

	tr := New()
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			tr.RecordError("bash")
		}()
	}
	wg.Wait()
	assert.Equal(t, goroutines, tr.Count("bash"))
}
