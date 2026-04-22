package memory_test

import (
	"strings"
	"testing"

	"github.com/dotcommander/piglet/extensions/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMemoryPrompt_OrdersByImportance(t *testing.T) {
	t.Parallel()

	s, err := memory.NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Clear() })

	require.NoError(t, s.Set("a", "low", ""))
	require.NoError(t, s.SetImportance("a", 1))
	require.NoError(t, s.Set("b", "high", ""))
	require.NoError(t, s.SetImportance("b", 10))
	require.NoError(t, s.Set("c", "mid", ""))
	require.NoError(t, s.SetImportance("c", 5))

	out := memory.BuildMemoryPrompt(s)

	posB := strings.Index(out, "- b:")
	posC := strings.Index(out, "- c:")
	posA := strings.Index(out, "- a:")

	assert.True(t, posB >= 0, "fact b missing from output")
	assert.True(t, posC >= 0, "fact c missing from output")
	assert.True(t, posA >= 0, "fact a missing from output")
	assert.Less(t, posB, posC, "b (importance 10) should appear before c (importance 5)")
	assert.Less(t, posC, posA, "c (importance 5) should appear before a (importance 1)")
}

func TestBuildMemoryPrompt_DropsLowestImportanceWhenOverCap(t *testing.T) {
	t.Parallel()

	s, err := memory.NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Clear() })

	// Fill store with ~200 facts, each ~50 chars of value → ~10 000 chars total > 7500 userCap.
	for i := range 200 {
		key := "fact-" + padInt(i, 3)
		val := strings.Repeat("x", 50)
		require.NoError(t, s.Set(key, val, ""))
		require.NoError(t, s.SetImportance(key, i)) // fact-199 has highest importance
	}

	// Add a clearly low-importance fact that should be dropped.
	require.NoError(t, s.Set("drop-me", strings.Repeat("y", 50), ""))
	require.NoError(t, s.SetImportance("drop-me", 0))

	// Add a clearly high-importance fact that must survive.
	require.NoError(t, s.Set("keep-me", "critical", ""))
	require.NoError(t, s.SetImportance("keep-me", 999))

	out := memory.BuildMemoryPrompt(s)

	assert.Contains(t, out, "keep-me", "highest-importance fact must survive cap trim")
	assert.NotContains(t, out, "drop-me", "lowest-importance fact must be dropped when over cap")
}

// padInt returns n formatted with at least width digits.
func padInt(n, width int) string {
	s := strings.Repeat("0", width) + itoa(n)
	return s[len(s)-width:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
