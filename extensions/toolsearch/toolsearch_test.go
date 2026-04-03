package toolsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitNames_Empty(t *testing.T) {
	t.Parallel()
	got := splitNames("")
	// Split on empty string yields one empty element
	require.Len(t, got, 1)
	assert.Equal(t, "", got[0])
}

func TestSplitNames_Single(t *testing.T) {
	t.Parallel()
	got := splitNames("tool_search")
	require.Len(t, got, 1)
	assert.Equal(t, "tool_search", got[0])
}

func TestSplitNames_Multiple(t *testing.T) {
	t.Parallel()
	got := splitNames("tool_a,tool_b,tool_c")
	require.Len(t, got, 3)
	// Results should be sorted
	assert.Equal(t, "tool_a", got[0])
	assert.Equal(t, "tool_b", got[1])
	assert.Equal(t, "tool_c", got[2])
}

func TestSplitNames_Sorted(t *testing.T) {
	t.Parallel()
	got := splitNames("zebra,apple,mango")
	require.Len(t, got, 3)
	assert.Equal(t, "apple", got[0])
	assert.Equal(t, "mango", got[1])
	assert.Equal(t, "zebra", got[2])
}

func TestSplitNames_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	got := splitNames(" tool_a , tool_b , tool_c ")
	require.Len(t, got, 3)
	for _, name := range got {
		assert.Equal(t, name, got[indexOf(got, name)], "names should not have leading/trailing spaces")
	}
	// All trimmed
	assert.Equal(t, "tool_a", got[indexOf(got, "tool_a")])
	assert.Equal(t, "tool_b", got[indexOf(got, "tool_b")])
	assert.Equal(t, "tool_c", got[indexOf(got, "tool_c")])
}

func TestSplitNames_LeadingTrailingCommas(t *testing.T) {
	t.Parallel()
	// Extra commas produce empty string entries
	got := splitNames("tool_a,,tool_b")
	require.Len(t, got, 3)
	// Empty string sorts before letters
	assert.Equal(t, "", got[0])
}

func TestSplitNames_SingleWithSpaces(t *testing.T) {
	t.Parallel()
	got := splitNames("  my_tool  ")
	require.Len(t, got, 1)
	assert.Equal(t, "my_tool", got[0])
}

func TestSplitNames_AlreadySorted(t *testing.T) {
	t.Parallel()
	got := splitNames("aaa,bbb,ccc")
	require.Len(t, got, 3)
	assert.Equal(t, "aaa", got[0])
	assert.Equal(t, "bbb", got[1])
	assert.Equal(t, "ccc", got[2])
}

func TestSplitNames_ReverseSorted(t *testing.T) {
	t.Parallel()
	got := splitNames("ccc,bbb,aaa")
	require.Len(t, got, 3)
	assert.Equal(t, "aaa", got[0])
	assert.Equal(t, "bbb", got[1])
	assert.Equal(t, "ccc", got[2])
}

func TestSplitNames_Duplicates(t *testing.T) {
	t.Parallel()
	got := splitNames("tool_a,tool_a,tool_b")
	require.Len(t, got, 3)
	// Duplicates are preserved (no dedup in splitNames)
	assert.Equal(t, "tool_a", got[0])
	assert.Equal(t, "tool_a", got[1])
	assert.Equal(t, "tool_b", got[2])
}

// indexOf is a test helper to find the index of s in slice.
func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}
