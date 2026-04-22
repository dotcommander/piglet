package session_test

import (
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetLeaf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		append func(t *testing.T, s *session.Session)
	}{
		{
			name:   "empty session",
			append: func(t *testing.T, s *session.Session) {},
		},
		{
			name: "single user message",
			append: func(t *testing.T, s *session.Session) {
				require.NoError(t, s.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))
			},
		},
		{
			name: "user plus assistant",
			append: func(t *testing.T, s *session.Session) {
				require.NoError(t, s.Append(&core.UserMessage{Content: "hi", Timestamp: time.Now()}))
				require.NoError(t, s.Append(&core.AssistantMessage{
					Content:    []core.AssistantContent{core.TextContent{Text: "response"}},
					StopReason: core.StopReasonStop,
					Timestamp:  time.Now(),
				}))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			s, err := session.New(dir, "/tmp")
			require.NoError(t, err)
			defer func() { _ = s.Close() }()

			tc.append(t, s)
			require.NoError(t, s.ResetLeaf())
			assert.Empty(t, s.Messages())
		})
	}
}

func TestResetLeafPersists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.Append(&core.UserMessage{Content: "original", Timestamp: time.Now()}))
	originalID := s.EntryInfos()[0].ID

	require.NoError(t, s.ResetLeaf())
	path := s.Path()
	require.NoError(t, s.Close())

	s2, err := session.Open(path)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	assert.Empty(t, s2.Messages())

	// Original entry still present in the full tree as a sibling root.
	var foundOriginal bool
	for _, n := range s2.FullTree() {
		if n.ID == originalID {
			foundOriginal = true
			assert.Equal(t, "user", n.Type)
			assert.Equal(t, 0, n.Depth)
		}
	}
	assert.True(t, foundOriginal, "original user entry should survive ResetLeaf")
}

func TestResetThenAppendCreatesSibling(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	require.NoError(t, s.Append(&core.UserMessage{Content: "first", Timestamp: time.Now()}))
	require.NoError(t, s.ResetLeaf())
	require.NoError(t, s.Append(&core.UserMessage{Content: "second", Timestamp: time.Now()}))

	msgs := s.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "second", msgs[0].(*core.UserMessage).Content)

	// Tree structure: "first" is a depth-0 user root from the original trunk.
	// ResetLeaf wrote a branch_summary as a sibling root; "second" is its child.
	tree := s.FullTree()
	var firstFound, summaryFound, secondFound bool
	for _, n := range tree {
		switch {
		case n.Type == "user" && n.Preview == "first" && n.Depth == 0:
			firstFound = true
		case n.Type == "branch_summary" && n.Depth == 0:
			summaryFound = true
		case n.Type == "user" && n.Preview == "second" && n.OnActivePath:
			secondFound = true
		}
	}
	assert.True(t, firstFound, "original user root should remain at depth 0")
	assert.True(t, summaryFound, "ResetLeaf should produce a sibling branch_summary root")
	assert.True(t, secondFound, "new user message should be on the active path")
}
