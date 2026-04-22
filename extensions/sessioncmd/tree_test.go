package sessioncmd

import (
	"testing"

	"github.com/dotcommander/piglet/sdk"
	"github.com/stretchr/testify/assert"
)

func TestNodeLabelByType(t *testing.T) {
	t.Parallel()

	ts := "2024-01-02T15:04:05Z"
	wantTS := "15:04:05"

	tests := []struct {
		name     string
		node     sdk.TreeNode
		contains string
	}{
		{
			name:     "user with preview",
			node:     sdk.TreeNode{Type: "user", Timestamp: ts, Preview: "hello world"},
			contains: "[user] hello world",
		},
		{
			name:     "user empty preview",
			node:     sdk.TreeNode{Type: "user", Timestamp: ts, Preview: ""},
			contains: "[user] (empty)",
		},
		{
			name:     "assistant with short preview",
			node:     sdk.TreeNode{Type: "assistant", Timestamp: ts, Preview: "short"},
			contains: "[asst] short",
		},
		{
			name:     "assistant empty preview",
			node:     sdk.TreeNode{Type: "assistant", Timestamp: ts, Preview: ""},
			contains: "[asst] (response)",
		},
		{
			name: "assistant preview truncated at 40 runes",
			node: sdk.TreeNode{
				Type:      "assistant",
				Timestamp: ts,
				Preview:   "1234567890123456789012345678901234567890EXTRA",
			},
			contains: "...",
		},
		{
			name:     "tool_result",
			node:     sdk.TreeNode{Type: "tool_result", Timestamp: ts},
			contains: "[tool]",
		},
		{
			name:     "compact",
			node:     sdk.TreeNode{Type: "compact", Timestamp: ts},
			contains: "[compact]",
		},
		{
			name:     "branch_summary",
			node:     sdk.TreeNode{Type: "branch_summary", Timestamp: ts},
			contains: "[branch]",
		},
		{
			name:     "custom_message with preview",
			node:     sdk.TreeNode{Type: "custom_message", Timestamp: ts, Preview: "injected text"},
			contains: "[msg] injected text",
		},
		{
			name:     "custom_message empty preview",
			node:     sdk.TreeNode{Type: "custom_message", Timestamp: ts, Preview: ""},
			contains: "[msg] (injected)",
		},
		{
			name:     "unknown type",
			node:     sdk.TreeNode{Type: "ext:memory:facts", Timestamp: ts},
			contains: "[ext:memory:facts]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := nodeLabel(tc.node)
			assert.Contains(t, got, tc.contains)
			assert.Contains(t, got, wantTS)
		})
	}
}

func TestNodeLabelAssistantTruncationBoundary(t *testing.T) {
	t.Parallel()

	// Exactly 40 runes — should NOT truncate
	preview40 := "12345678901234567890123456789012345678901234" // 44 runes — intentional: truncated to 40+...
	node := sdk.TreeNode{Type: "assistant", Timestamp: "2024-01-02T15:04:05Z", Preview: preview40}
	got := nodeLabel(node)
	assert.Contains(t, got, "...")

	// Exactly 40 runes — no truncation
	preview40exact := "1234567890123456789012345678901234567890" // exactly 40 runes
	node40 := sdk.TreeNode{Type: "assistant", Timestamp: "2024-01-02T15:04:05Z", Preview: preview40exact}
	got40 := nodeLabel(node40)
	assert.NotContains(t, got40, "...")
}

func TestNodeLabel_CompactWithTokens(t *testing.T) {
	t.Parallel()
	node := sdk.TreeNode{Type: "compact", Timestamp: "2024-01-02T15:04:05Z", TokensBefore: 12345}
	got := nodeLabel(node)
	assert.Contains(t, got, "tokens=12345")
	assert.Contains(t, got, "[compact")
}

func TestNodeLabel_CompactWithoutTokens(t *testing.T) {
	t.Parallel()
	node := sdk.TreeNode{Type: "compact", Timestamp: "2024-01-02T15:04:05Z", TokensBefore: 0}
	got := nodeLabel(node)
	assert.NotContains(t, got, "tokens=")
	assert.Contains(t, got, "[compact]")
}
