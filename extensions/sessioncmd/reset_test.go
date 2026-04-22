package sessioncmd

import (
	"testing"

	"github.com/dotcommander/piglet/sdk"
	"github.com/stretchr/testify/assert"
)

func TestFirstActiveUserPreview(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		nodes []sdk.TreeNode
		want  string
	}{
		{
			name:  "empty list",
			nodes: nil,
			want:  "",
		},
		{
			name: "single user on active path",
			nodes: []sdk.TreeNode{
				{Type: "user", OnActivePath: true, Depth: 0, Preview: "hello"},
			},
			want: "hello",
		},
		{
			name: "off-path user ignored",
			nodes: []sdk.TreeNode{
				{Type: "user", OnActivePath: false, Depth: 0, Preview: "off"},
				{Type: "user", OnActivePath: true, Depth: 1, Preview: "on"},
			},
			want: "on",
		},
		{
			name: "shallowest active user wins",
			nodes: []sdk.TreeNode{
				{Type: "assistant", OnActivePath: true, Depth: 0, Preview: "asst"},
				{Type: "user", OnActivePath: true, Depth: 1, Preview: "root-user"},
				{Type: "user", OnActivePath: true, Depth: 3, Preview: "nested-user"},
			},
			want: "root-user",
		},
		{
			name: "non-user types skipped",
			nodes: []sdk.TreeNode{
				{Type: "assistant", OnActivePath: true, Depth: 0, Preview: "a"},
				{Type: "tool_result", OnActivePath: true, Depth: 1, Preview: "tr"},
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := firstActiveUserPreview(tc.nodes)
			assert.Equal(t, tc.want, got)
		})
	}
}
