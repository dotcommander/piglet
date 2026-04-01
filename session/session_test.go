package session_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/session"
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
		require.NoError(t, s.Append(&core.UserMessage{Content: string(rune('a' + i)), Timestamp: time.Now()}))
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

func TestCompactRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	// Write 6 original messages
	for i := range 6 {
		require.NoError(t, s.Append(&core.UserMessage{Content: fmt.Sprintf("msg%d", i), Timestamp: time.Now()}))
	}

	// Compact to 2 messages (summary + last)
	compacted := []core.Message{
		&core.UserMessage{Content: "summary of 0-4", Timestamp: time.Now()},
		&core.UserMessage{Content: "msg5", Timestamp: time.Now()},
	}
	require.NoError(t, s.AppendCompact(compacted))

	// Write 1 post-compact message
	require.NoError(t, s.Append(&core.UserMessage{Content: "msg6", Timestamp: time.Now()}))

	path := s.Path()
	require.NoError(t, s.Close())

	// Reload — should see compacted state + post-compact message, not originals
	s2, err := session.Open(path)
	require.NoError(t, err)
	defer s2.Close()

	msgs := s2.Messages()
	require.Len(t, msgs, 3, "should have 2 compacted + 1 post-compact")
	assert.Equal(t, "summary of 0-4", msgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "msg5", msgs[1].(*core.UserMessage).Content)
	assert.Equal(t, "msg6", msgs[2].(*core.UserMessage).Content)
}

func TestCompactReplacesInMemoryMessages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "b", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "c", Timestamp: time.Now()}))
	require.Len(t, s.Messages(), 3)

	compacted := []core.Message{
		&core.UserMessage{Content: "summary", Timestamp: time.Now()},
	}
	require.NoError(t, s.AppendCompact(compacted))

	msgs := s.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "summary", msgs[0].(*core.UserMessage).Content)
}

func TestCompactScanSummary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	// Write 10 messages, compact to 3, then add 2 more
	for i := range 10 {
		require.NoError(t, s.Append(&core.UserMessage{Content: fmt.Sprintf("m%d", i), Timestamp: time.Now()}))
	}
	compacted := []core.Message{
		&core.UserMessage{Content: "s1", Timestamp: time.Now()},
		&core.UserMessage{Content: "s2", Timestamp: time.Now()},
		&core.UserMessage{Content: "s3", Timestamp: time.Now()},
	}
	require.NoError(t, s.AppendCompact(compacted))
	require.NoError(t, s.Append(&core.UserMessage{Content: "post1", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "post2", Timestamp: time.Now()}))
	require.NoError(t, s.Close())

	summaries, err := session.List(dir)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	// 3 from compact + 2 post-compact = 5
	assert.Equal(t, 5, summaries[0].Messages)
}

// ---------------------------------------------------------------------------
// In-place branching tests
// ---------------------------------------------------------------------------

func TestBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "b", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "c", Timestamp: time.Now()}))

	// Get entry info for the branch point
	infos := s.EntryInfos()
	require.Len(t, infos, 3)
	branchPoint := infos[0].ID // entry "a"

	// Branch back to first entry
	require.NoError(t, s.Branch(branchPoint))

	// Messages should now be just [a]
	msgs := s.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "a", msgs[0].(*core.UserMessage).Content)

	// Append on the new branch
	require.NoError(t, s.Append(&core.UserMessage{Content: "d", Timestamp: time.Now()}))
	msgs = s.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "a", msgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "d", msgs[1].(*core.UserMessage).Content)
}

func TestBranchWithSummary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	require.NoError(t, s.Append(&core.UserMessage{Content: "x", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "y", Timestamp: time.Now()}))

	infos := s.EntryInfos()
	require.Len(t, infos, 2)
	branchPoint := infos[0].ID

	require.NoError(t, s.BranchWithSummary(branchPoint, "tried Y but it failed"))

	// Summary is not a conversation message, so Messages() returns just [x]
	msgs := s.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "x", msgs[0].(*core.UserMessage).Content)

	// But the summary exists as an entry
	newInfos := s.EntryInfos()
	require.Len(t, newInfos, 2) // x + branch_summary
	assert.Equal(t, "branch_summary", newInfos[1].Type)
}

func TestBranchPersistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "b", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "c", Timestamp: time.Now()}))

	infos := s.EntryInfos()
	branchPoint := infos[0].ID // entry "a"

	require.NoError(t, s.Branch(branchPoint))
	require.NoError(t, s.Append(&core.UserMessage{Content: "d", Timestamp: time.Now()}))

	path := s.Path()
	require.NoError(t, s.Close())

	// Reload — should recover the branched state
	s2, err := session.Open(path)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	msgs := s2.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "a", msgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "d", msgs[1].(*core.UserMessage).Content)
}

func TestBranchWithCompaction(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	// Build initial conversation
	for i := range 5 {
		require.NoError(t, s.Append(&core.UserMessage{Content: fmt.Sprintf("m%d", i), Timestamp: time.Now()}))
	}

	// Compact
	require.NoError(t, s.AppendCompact([]core.Message{
		&core.UserMessage{Content: "summary", Timestamp: time.Now()},
	}))

	// Add post-compact messages
	require.NoError(t, s.Append(&core.UserMessage{Content: "post1", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "post2", Timestamp: time.Now()}))

	// Branch back to the compact entry
	infos := s.EntryInfos()
	var compactID string
	for _, info := range infos {
		if info.Type == "compact" {
			compactID = info.ID
			break
		}
	}
	require.NotEmpty(t, compactID)

	require.NoError(t, s.Branch(compactID))
	msgs := s.Messages()
	require.Len(t, msgs, 1) // just the compact summary
	assert.Equal(t, "summary", msgs[0].(*core.UserMessage).Content)

	// Append on new branch
	require.NoError(t, s.Append(&core.UserMessage{Content: "alt1", Timestamp: time.Now()}))
	msgs = s.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "summary", msgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "alt1", msgs[1].(*core.UserMessage).Content)
}

func TestBranchInvalidEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	err = s.Branch("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEntryInfos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Append(&core.UserMessage{Content: "hello world", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.AssistantMessage{
		Content:    []core.AssistantContent{core.TextContent{Text: "hi back"}},
		StopReason: core.StopReasonStop,
		Timestamp:  time.Now(),
	}))
	require.NoError(t, s.Append(&core.ToolResultMessage{
		ToolCallID: "tc1",
		ToolName:   "test",
		Content:    []core.ContentBlock{core.TextContent{Text: "result"}},
	}))

	infos := s.EntryInfos()
	require.Len(t, infos, 3)

	assert.Equal(t, "user", infos[0].Type)
	assert.Equal(t, "assistant", infos[1].Type)
	assert.Equal(t, "tool_result", infos[2].Type)
}

func TestLeafID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	assert.Empty(t, s.LeafID()) // no entries yet

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	leaf1 := s.LeafID()
	assert.NotEmpty(t, leaf1)

	require.NoError(t, s.Append(&core.UserMessage{Content: "b", Timestamp: time.Now()}))
	leaf2 := s.LeafID()
	assert.NotEqual(t, leaf1, leaf2)

	// Branch back — leaf becomes a new branch_summary entry, not leaf1 itself
	require.NoError(t, s.Branch(leaf1))
	leaf3 := s.LeafID()
	assert.NotEqual(t, leaf1, leaf3, "branch writes a summary entry")
	assert.NotEqual(t, leaf2, leaf3)

	// Messages should show only [a] (branch_summary is not a conversation message)
	msgs := s.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "a", msgs[0].(*core.UserMessage).Content)
}

func TestBranchSummaryNotInScanCount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "b", Timestamp: time.Now()}))

	infos := s.EntryInfos()
	require.NoError(t, s.BranchWithSummary(infos[0].ID, "abandoned"))
	require.NoError(t, s.Append(&core.UserMessage{Content: "c", Timestamp: time.Now()}))
	require.NoError(t, s.Close())

	summaries, err := session.List(dir)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	// 3 message entries (a, b, c), branch_summary should NOT be counted
	assert.Equal(t, 3, summaries[0].Messages)
}

// ---------------------------------------------------------------------------
// AppendCustom tests
// ---------------------------------------------------------------------------

func TestAppendCustom(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	// Append a regular message first
	require.NoError(t, s.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))

	// Append a custom entry
	require.NoError(t, s.AppendCustom("ext:test:state", map[string]string{"key": "value"}))

	// Custom entry should not appear in Messages (it's metadata)
	msgs := s.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "hello", msgs[0].(*core.UserMessage).Content)

	// But it should appear in EntryInfos
	infos := s.EntryInfos()
	require.Len(t, infos, 2)
	assert.Equal(t, "user", infos[0].Type)
	assert.Equal(t, "ext:test:state", infos[1].Type)
}

func TestAppendCustomPersistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))
	require.NoError(t, s.AppendCustom("ext:test:data", map[string]int{"count": 42}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "world", Timestamp: time.Now()}))

	path := s.Path()
	require.NoError(t, s.Close())

	// Reload — custom entry should be in the tree, messages intact
	s2, err := session.Open(path)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	msgs := s2.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "hello", msgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "world", msgs[1].(*core.UserMessage).Content)
}

func TestAppendCustomBranchCorrectly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	require.NoError(t, s.AppendCustom("ext:test:marker", "checkpoint"))
	require.NoError(t, s.Append(&core.UserMessage{Content: "b", Timestamp: time.Now()}))

	// Branch back to entry "a"
	infos := s.EntryInfos()
	require.NoError(t, s.Branch(infos[0].ID))

	// Custom entry should NOT be in the new branch's path
	msgs := s.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "a", msgs[0].(*core.UserMessage).Content)
}

// ---------------------------------------------------------------------------
// AppendCustomMessage tests
// ---------------------------------------------------------------------------

func TestAppendCustomMessage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))
	require.NoError(t, s.AppendCustomMessage("assistant", "injected context"))
	require.NoError(t, s.Append(&core.UserMessage{Content: "world", Timestamp: time.Now()}))

	// Custom message SHOULD appear in Messages (unlike AppendCustom)
	msgs := s.Messages()
	require.Len(t, msgs, 3)
	assert.Equal(t, "hello", msgs[0].(*core.UserMessage).Content)

	am, ok := msgs[1].(*core.AssistantMessage)
	require.True(t, ok, "custom_message with role=assistant should produce AssistantMessage")
	require.Len(t, am.Content, 1)
	tc, ok := am.Content[0].(core.TextContent)
	require.True(t, ok)
	assert.Equal(t, "injected context", tc.Text)

	assert.Equal(t, "world", msgs[2].(*core.UserMessage).Content)
}

func TestAppendCustomMessagePersistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.AppendCustomMessage("user", "injected user msg"))
	require.NoError(t, s.AppendCustomMessage("assistant", "injected assistant msg"))

	path := s.Path()
	require.NoError(t, s.Close())

	// Reload — custom messages should survive and appear in Messages()
	s2, err := session.Open(path)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	msgs := s2.Messages()
	require.Len(t, msgs, 2)

	um, ok := msgs[0].(*core.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "injected user msg", um.Content)

	am, ok := msgs[1].(*core.AssistantMessage)
	require.True(t, ok)
	require.Len(t, am.Content, 1)
	tc, ok := am.Content[0].(core.TextContent)
	require.True(t, ok)
	assert.Equal(t, "injected assistant msg", tc.Text)
}

func TestAppendCustomMessageInScanCount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	require.NoError(t, s.AppendCustomMessage("assistant", "injected"))
	require.NoError(t, s.Close())

	summaries, err := session.List(dir)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, 2, summaries[0].Messages) // both user and custom_message counted
}

// ---------------------------------------------------------------------------
// FullTree tests
// ---------------------------------------------------------------------------

func TestFullTree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "b", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "c", Timestamp: time.Now()}))

	tree := s.FullTree()
	require.Len(t, tree, 3)

	// All should be on active path (linear chain)
	for _, n := range tree {
		assert.True(t, n.OnActivePath, "node %s should be on active path", n.ID)
	}

	// Depth should increase
	assert.Equal(t, 0, tree[0].Depth)
	assert.Equal(t, 1, tree[1].Depth)
	assert.Equal(t, 2, tree[2].Depth)

	// Previews
	assert.Equal(t, "a", tree[0].Preview)
	assert.Equal(t, "b", tree[1].Preview)
	assert.Equal(t, "c", tree[2].Preview)
}

func TestFullTreeWithBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Append(&core.UserMessage{Content: "a", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "b", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "c", Timestamp: time.Now()}))

	// Branch back to "a"
	infos := s.EntryInfos()
	require.NoError(t, s.Branch(infos[0].ID))
	require.NoError(t, s.Append(&core.UserMessage{Content: "d", Timestamp: time.Now()}))

	tree := s.FullTree()
	// a → {b → c, branch_summary → d}
	// Total: a, branch_summary, d, b, c = 5 nodes
	require.Len(t, tree, 5)

	// Root "a" should be first and on active path
	assert.Equal(t, "a", tree[0].Preview)
	assert.True(t, tree[0].OnActivePath)

	// Active subtree should come before abandoned branch
	// After "a": branch_summary (active), d (active), then b (inactive), c (inactive)
	assert.True(t, tree[1].OnActivePath)  // branch_summary
	assert.True(t, tree[2].OnActivePath)  // d
	assert.False(t, tree[3].OnActivePath) // b (abandoned)
	assert.False(t, tree[4].OnActivePath) // c (abandoned)
}

func TestAppendLabel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))

	infos := s.EntryInfos()
	require.Len(t, infos, 1)
	targetID := infos[0].ID

	require.NoError(t, s.AppendLabel(targetID, "checkpoint-1"))
	assert.Equal(t, "checkpoint-1", s.Label(targetID))

	// Label appears in FullTree on the target node
	tree := s.FullTree()
	require.Len(t, tree, 2) // user message + label entry
	assert.Equal(t, "checkpoint-1", tree[0].Label)
	assert.Equal(t, "label", tree[1].Type)
}

func TestAppendLabelClear(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))

	infos := s.EntryInfos()
	targetID := infos[0].ID

	require.NoError(t, s.AppendLabel(targetID, "checkpoint-1"))
	assert.Equal(t, "checkpoint-1", s.Label(targetID))

	// Clear
	require.NoError(t, s.AppendLabel(targetID, ""))
	assert.Equal(t, "", s.Label(targetID))
}

func TestAppendLabelPersistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	require.NoError(t, s.Append(&core.UserMessage{Content: "hello", Timestamp: time.Now()}))

	infos := s.EntryInfos()
	targetID := infos[0].ID

	require.NoError(t, s.AppendLabel(targetID, "bookmark"))

	path := s.Path()
	require.NoError(t, s.Close())

	// Reopen and verify label survives
	s2, err := session.Open(path)
	require.NoError(t, err)
	defer s2.Close()

	assert.Equal(t, "bookmark", s2.Label(targetID))

	tree := s2.FullTree()
	require.Len(t, tree, 2) // user message + label entry
	assert.Equal(t, "bookmark", tree[0].Label)
}

func TestCompactMultiple(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	// First batch
	for i := range 4 {
		require.NoError(t, s.Append(&core.UserMessage{Content: fmt.Sprintf("a%d", i), Timestamp: time.Now()}))
	}
	require.NoError(t, s.AppendCompact([]core.Message{
		&core.UserMessage{Content: "compact1", Timestamp: time.Now()},
	}))

	// Second batch + second compaction
	require.NoError(t, s.Append(&core.UserMessage{Content: "b0", Timestamp: time.Now()}))
	require.NoError(t, s.Append(&core.UserMessage{Content: "b1", Timestamp: time.Now()}))
	require.NoError(t, s.AppendCompact([]core.Message{
		&core.UserMessage{Content: "compact2", Timestamp: time.Now()},
	}))

	// Post-compact message
	require.NoError(t, s.Append(&core.UserMessage{Content: "final", Timestamp: time.Now()}))

	path := s.Path()
	require.NoError(t, s.Close())

	// Reload — only last compact + post-compact messages survive
	s2, err := session.Open(path)
	require.NoError(t, err)
	defer s2.Close()

	msgs := s2.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "compact2", msgs[0].(*core.UserMessage).Content)
	assert.Equal(t, "final", msgs[1].(*core.UserMessage).Content)
}
