package session_test

import (
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/session"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/home/user")
	require.NoError(t, err)
	defer s.Close()

	assert.NotEmpty(t, s.ID())
	assert.NotEmpty(t, s.Path())
	assert.Equal(t, "/home/user", s.Meta().CWD)
	assert.Empty(t, s.Messages())
}

func TestAppendAndReload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create and populate
	s1, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s1.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))
	require.NoError(t, s1.Append(&core.AssistantMessage{
		Content:    []core.AssistantContent{core.TextContent{Text: "hi there"}},
		StopReason: core.StopReasonStop,
		Timestamp:  time.Now(),
	}))

	path := s1.Path()
	id := s1.ID()
	require.NoError(t, s1.Close())

	// Reload
	s2, err := session.Open(path)
	require.NoError(t, err)
	defer s2.Close()

	assert.Equal(t, id, s2.ID())
	msgs := s2.Messages()
	require.Len(t, msgs, 2)

	um, ok := msgs[0].(*core.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "hello", um.Content)

	am, ok := msgs[1].(*core.AssistantMessage)
	require.True(t, ok)
	require.Len(t, am.Content, 1)
	assert.Equal(t, "hi there", am.Content[0].(core.TextContent).Text)
}

func TestToolResultPersistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.Append(&core.ToolResultMessage{
		ToolCallID: "tc1",
		ToolName:   "echo",
		Content:    []core.ContentBlock{core.TextContent{Text: "echoed"}},
		IsError:    false,
	}))

	path := s.Path()
	require.NoError(t, s.Close())

	s2, err := session.Open(path)
	require.NoError(t, err)
	defer s2.Close()

	msgs := s2.Messages()
	require.Len(t, msgs, 1)
	tr, ok := msgs[0].(*core.ToolResultMessage)
	require.True(t, ok)
	assert.Equal(t, "tc1", tr.ToolCallID)
	assert.Equal(t, "echo", tr.ToolName)
}

func TestSetTitle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.SetTitle("My Session"))
	path := s.Path()
	require.NoError(t, s.Close())

	s2, err := session.Open(path)
	require.NoError(t, err)
	defer s2.Close()

	assert.Equal(t, "My Session", s2.Meta().Title)
}

func TestFork(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	for i := range 5 {
		require.NoError(t, s.Append(&core.UserMessage{Content: string(rune('a'+i)), Timestamp: time.Now()}))
	}

	// Fork keeping first 3 messages
	forked, err := s.Fork(3)
	require.NoError(t, err)
	defer forked.Close()
	defer s.Close()

	assert.NotEqual(t, s.ID(), forked.ID())
	assert.Len(t, forked.Messages(), 3)

	// Verify messages
	msgs := forked.Messages()
	assert.Equal(t, "a", msgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "b", msgs[1].(*core.UserMessage).Content)
	assert.Equal(t, "c", msgs[2].(*core.UserMessage).Content)
}

func TestForkMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	for i := range 5 {
		require.NoError(t, s.Append(&core.UserMessage{Content: string(rune('a' + i)), Timestamp: time.Now()}))
	}

	forked, err := s.Fork(3)
	require.NoError(t, err)
	defer forked.Close()
	defer s.Close()

	// Verify branch metadata
	assert.Equal(t, s.ID(), forked.Meta().ParentID, "forked session should reference parent ID")
	assert.Equal(t, 3, forked.Meta().ForkPoint, "fork point should match keepMessages")

	// Verify parent has no branch metadata
	assert.Empty(t, s.Meta().ParentID)
	assert.Zero(t, s.Meta().ForkPoint)
}

func TestForkMetadataPersistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	require.NoError(t, s.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))

	forked, err := s.Fork(1)
	require.NoError(t, err)
	forkedPath := forked.Path()
	parentID := s.ID()
	require.NoError(t, forked.Close())
	require.NoError(t, s.Close())

	// Reload forked session — metadata should survive
	reloaded, err := session.Open(forkedPath)
	require.NoError(t, err)
	defer reloaded.Close()

	assert.Equal(t, parentID, reloaded.Meta().ParentID)
	assert.Equal(t, 1, reloaded.Meta().ForkPoint)
}

func TestForkIndependentHistories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	require.NoError(t, s.Append(&core.UserMessage{Content: "shared", Timestamp: time.Now()}))

	forked, err := s.Fork(1)
	require.NoError(t, err)

	// Append different messages to each
	require.NoError(t, s.Append(&core.UserMessage{Content: "parent-only", Timestamp: time.Now()}))
	require.NoError(t, forked.Append(&core.UserMessage{Content: "fork-only", Timestamp: time.Now()}))

	defer s.Close()
	defer forked.Close()

	parentMsgs := s.Messages()
	forkMsgs := forked.Messages()

	require.Len(t, parentMsgs, 2)
	require.Len(t, forkMsgs, 2)

	assert.Equal(t, "shared", parentMsgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "parent-only", parentMsgs[1].(*core.UserMessage).Content)
	assert.Equal(t, "shared", forkMsgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "fork-only", forkMsgs[1].(*core.UserMessage).Content)
}

func TestListShowsParentID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	require.NoError(t, s.Append(&core.UserMessage{Content: "msg", Timestamp: time.Now()}))

	forked, err := s.Fork(1)
	require.NoError(t, err)
	require.NoError(t, forked.Close())
	require.NoError(t, s.Close())

	summaries, err := session.List(dir)
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	// Find the forked session in summaries
	var found bool
	for _, sum := range summaries {
		if sum.ParentID != "" {
			assert.Equal(t, s.ID(), sum.ParentID)
			found = true
		}
	}
	assert.True(t, found, "should find a forked session with ParentID")
}

func TestForkAll(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.Append(&core.UserMessage{Content: "only", Timestamp: time.Now()}))

	forked, err := s.Fork(0) // 0 = keep all
	require.NoError(t, err)
	defer forked.Close()
	defer s.Close()

	assert.Len(t, forked.Messages(), 1)
}

func TestList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create two sessions
	s1, err := session.New(dir, "/project1")
	require.NoError(t, err)
	require.NoError(t, s1.SetTitle("First"))
	require.NoError(t, s1.Append(&core.UserMessage{Content: "msg1", Timestamp: time.Now()}))
	require.NoError(t, s1.Close())

	time.Sleep(10 * time.Millisecond) // ensure different timestamps

	s2, err := session.New(dir, "/project2")
	require.NoError(t, err)
	require.NoError(t, s2.SetTitle("Second"))
	require.NoError(t, s2.Append(&core.UserMessage{Content: "msg2", Timestamp: time.Now()}))
	require.NoError(t, s2.Append(&core.UserMessage{Content: "msg3", Timestamp: time.Now()}))
	require.NoError(t, s2.Close())

	summaries, err := session.List(dir)
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	// Newest first
	assert.Equal(t, "Second", summaries[0].Title)
	assert.Equal(t, 2, summaries[0].Messages)
	assert.Equal(t, "First", summaries[1].Title)
	assert.Equal(t, 1, summaries[1].Messages)
}

func TestList_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	summaries, err := session.List(dir)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestList_NonExistent(t *testing.T) {
	t.Parallel()

	summaries, err := session.List("/nonexistent/dir/12345")
	require.NoError(t, err)
	assert.Nil(t, summaries)
}
