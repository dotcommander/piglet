package session_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppendCompact_StoresTokensBefore verifies round-trip: a compact entry
// written with tokensBefore=12345 survives a close/open cycle and appears in
// FullTree with TokensBefore==12345.
func TestAppendCompact_StoresTokensBefore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	msgs := []core.Message{
		&core.UserMessage{Content: "summary", Timestamp: time.Now()},
	}
	require.NoError(t, s.AppendCompact(msgs, 12345))

	path := s.Path()
	require.NoError(t, s.Close())

	s2, err := session.Open(path)
	require.NoError(t, err)
	defer s2.Close()

	nodes := s2.FullTree()
	var compact *session.TreeNode
	for i := range nodes {
		if nodes[i].Type == "compact" {
			compact = &nodes[i]
			break
		}
	}
	require.NotNil(t, compact, "compact node must exist in FullTree")
	assert.Equal(t, 12345, compact.TokensBefore)
}

// TestAppendCompact_ZeroTokensAbsentFromJSON verifies that tokensBefore=0
// does not appear in the raw JSONL (omitempty semantics).
func TestAppendCompact_ZeroTokensAbsentFromJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	msgs := []core.Message{
		&core.UserMessage{Content: "summary", Timestamp: time.Now()},
	}
	require.NoError(t, s.AppendCompact(msgs, 0))

	path := s.Path()
	require.NoError(t, s.Close())

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		typeVal := strings.Trim(string(entry["type"]), `"`)
		if typeVal == "compact" {
			assert.NotContains(t, line, `"tokensBefore"`, "tokensBefore must be absent when zero")
		}
	}
}

// TestFullTree_BackCompatMissingTokensBefore verifies that a compact entry
// without the tokensBefore field (old JSONL format) loads with TokensBefore==0
// and produces no error.
func TestFullTree_BackCompatMissingTokensBefore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write a session with a compact entry that has no tokensBefore field.
	s, err := session.New(dir, "/tmp")
	require.NoError(t, err)

	msgs := []core.Message{
		&core.UserMessage{Content: "old-format-summary", Timestamp: time.Now()},
	}
	// Write with tokensBefore=12345 first, then strip the field from the file.
	require.NoError(t, s.AppendCompact(msgs, 12345))
	path := s.Path()
	require.NoError(t, s.Close())

	// Strip tokensBefore from the raw JSONL to simulate an old-format file.
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	patched := strings.ReplaceAll(string(raw), `,"tokensBefore":12345`, "")
	patched = strings.ReplaceAll(patched, `"tokensBefore":12345,`, "")
	require.NoError(t, os.WriteFile(path, []byte(patched), 0o600))

	s2, err := session.Open(path)
	require.NoError(t, err)
	defer s2.Close()

	nodes := s2.FullTree()
	var compact *session.TreeNode
	for i := range nodes {
		if nodes[i].Type == "compact" {
			compact = &nodes[i]
			break
		}
	}
	require.NotNil(t, compact, "compact node must exist after back-compat load")
	assert.Equal(t, 0, compact.TokensBefore, "missing field must deserialize as 0")
}
