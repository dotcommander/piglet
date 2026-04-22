package sessioncmd

import (
	"testing"

	"github.com/dotcommander/piglet/sdk"
	"github.com/stretchr/testify/assert"
)

func TestCurrentLeafID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		nodes     []sdk.TreeNode
		wantID    string
		wantFound bool
	}{
		{
			name: "single leaf on active path",
			nodes: []sdk.TreeNode{
				{ID: "root", Depth: 0, OnActivePath: true},
				{ID: "mid", Depth: 1, OnActivePath: true},
				{ID: "leaf", Depth: 2, OnActivePath: true},
				{ID: "branch", Depth: 1, OnActivePath: false},
			},
			wantID:    "leaf",
			wantFound: true,
		},
		{
			name: "no active path nodes",
			nodes: []sdk.TreeNode{
				{ID: "a", Depth: 0, OnActivePath: false},
				{ID: "b", Depth: 1, OnActivePath: false},
			},
			wantID:    "",
			wantFound: false,
		},
		{
			name: "multiple active-path nodes — deepest wins",
			nodes: []sdk.TreeNode{
				{ID: "shallow", Depth: 0, OnActivePath: true},
				{ID: "deeper", Depth: 3, OnActivePath: true},
				{ID: "middle", Depth: 1, OnActivePath: true},
				{ID: "off", Depth: 5, OnActivePath: false},
			},
			wantID:    "deeper",
			wantFound: true,
		},
		{
			name:      "empty node list",
			nodes:     []sdk.TreeNode{},
			wantID:    "",
			wantFound: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotID, gotFound := currentLeafID(tc.nodes)
			assert.Equal(t, tc.wantFound, gotFound)
			assert.Equal(t, tc.wantID, gotID)
		})
	}
}
