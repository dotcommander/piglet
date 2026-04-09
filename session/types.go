package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/dotcommander/piglet/core"
)

// Entry type constants.
const (
	entryTypeUser          = "user"
	entryTypeAssistant     = "assistant"
	entryTypeToolResult    = "tool_result"
	entryTypeMeta          = "meta"
	entryTypeCompact       = "compact"
	entryTypeBranchSummary = "branch_summary"
	entryTypeCustomMessage = "custom_message"
	entryTypeLabel         = "label"
)

// Entry is a single line in the JSONL session file.
type Entry struct {
	Type      string          `json:"type"`               // "user", "assistant", "tool_result", "meta", "compact", "branch_summary"
	ID        string          `json:"id,omitempty"`       // 8-char hex; empty for meta and legacy entries
	ParentID  string          `json:"parentId,omitempty"` // parent entry ID; empty for first entry
	Timestamp time.Time       `json:"ts"`
	Data      json.RawMessage `json:"data"`
}

// BranchSummaryData holds the data payload for a branch_summary entry.
type BranchSummaryData struct {
	Summary string `json:"summary"`
	FromID  string `json:"fromId"` // leaf ID of the abandoned branch
}

// CustomMessageData holds the data payload for a custom_message entry.
type CustomMessageData struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// LabelData holds the data payload for a label entry.
type LabelData struct {
	TargetID string `json:"targetId"` // entry ID being labeled
	Label    string `json:"label"`    // empty = clear label
}

// EntryInfo is a public view of a tree node for display and navigation.
type EntryInfo struct {
	ID        string
	ParentID  string
	Type      string
	Timestamp time.Time
	Children  int
}

// TreeNode is a full-tree view of a session entry for DAG rendering.
type TreeNode struct {
	ID           string
	ParentID     string
	Type         string
	Timestamp    time.Time
	Children     int
	OnActivePath bool
	Depth        int
	Preview      string // truncated content for user messages
	Label        string // user-assigned bookmark label (empty if none)
}

// Meta holds session metadata.
type Meta struct {
	ID        string    `json:"id"`
	CWD       string    `json:"cwd"`
	Model     string    `json:"model,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Title     string    `json:"title,omitempty"`
	ParentID  string    `json:"parentId,omitzero"`
	ForkPoint int       `json:"forkPoint,omitzero"`
}

// Summary is returned by List.
type Summary struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	CWD       string    `json:"cwd"`
	CreatedAt time.Time `json:"createdAt"`
	Messages  int       `json:"messages"`
	ParentID  string    `json:"parentId,omitzero"`
}

// node is an in-memory tree node.
type node struct {
	parentID string
	typ      string
	ts       time.Time
	message  core.Message   // non-nil for user/assistant/tool_result
	compact  []core.Message // non-nil for compact entries
}

// generateEntryID returns a random 8-character hex ID.
func generateEntryID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
