package sessioncmd

import (
	"testing"

	"github.com/dotcommander/piglet/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		id    string
		want  string
	}{
		{
			name:  "non-empty title returned as-is",
			title: "My Session",
			id:    "abc12345",
			want:  "My Session",
		},
		{
			name:  "empty title long ID truncated to 8",
			title: "",
			id:    "abc123456789",
			want:  "abc12345",
		},
		{
			name:  "empty title short ID returned as-is",
			title: "",
			id:    "abc",
			want:  "abc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, sessionLabel(tc.title, tc.id))
		})
	}
}

func TestBuildSessionTree(t *testing.T) {
	t.Parallel()

	t.Run("multiple roots no children", func(t *testing.T) {
		t.Parallel()
		summaries := []sdk.SessionInfo{
			{ID: "a"},
			{ID: "b"},
		}
		tree := buildSessionTree(summaries)
		assert.Len(t, tree.roots, 2)
		assert.Empty(t, tree.children)
	})

	t.Run("parent child linking", func(t *testing.T) {
		t.Parallel()
		summaries := []sdk.SessionInfo{
			{ID: "parent"},
			{ID: "child", ParentID: "parent"},
		}
		tree := buildSessionTree(summaries)
		require.Len(t, tree.roots, 1)
		assert.Equal(t, 0, tree.roots[0]) // "parent" at index 0
		kids := tree.children["parent"]
		require.Len(t, kids, 1)
		assert.Equal(t, 1, kids[0]) // "child" at index 1
	})

	t.Run("orphaned parent becomes root", func(t *testing.T) {
		t.Parallel()
		summaries := []sdk.SessionInfo{
			{ID: "child", ParentID: "missing-parent"},
		}
		tree := buildSessionTree(summaries)
		assert.Len(t, tree.roots, 1)
		assert.Empty(t, tree.children)
	})

	t.Run("multiple children same parent", func(t *testing.T) {
		t.Parallel()
		summaries := []sdk.SessionInfo{
			{ID: "root"},
			{ID: "child1", ParentID: "root"},
			{ID: "child2", ParentID: "root"},
		}
		tree := buildSessionTree(summaries)
		require.Len(t, tree.roots, 1)
		assert.Len(t, tree.children["root"], 2)
	})
}
