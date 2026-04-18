package sdk

import (
	"context"
	"encoding/json"
)

// ConversationMessages returns the current conversation history as raw JSON.
func (e *Extension) ConversationMessages(ctx context.Context) (json.RawMessage, error) {
	type resp struct {
		Messages json.RawMessage `json:"messages"`
	}
	r, err := hostCall[resp](e, ctx, "host/conversationMessages", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Messages, nil
}

// LLMSnapshot returns a read-only projection of what would be sent to the LLM:
// system prompt, conversation messages, and tool schemas.
func (e *Extension) LLMSnapshot(ctx context.Context) (LLMSnapshotResult, error) {
	return hostCall[LLMSnapshotResult](e, ctx, "host/llmSnapshot", struct{}{})
}

// LLMSnapshotResult is the response from host/llmSnapshot.
type LLMSnapshotResult struct {
	System   string          `json:"system"`
	Messages json.RawMessage `json:"messages"`
	Tools    json.RawMessage `json:"tools"`
}

// SetConversationMessages replaces the conversation history with the given wire messages.
func (e *Extension) SetConversationMessages(ctx context.Context, messages json.RawMessage) error {
	return hostCallVoid(e, ctx, "host/setConversationMessages", map[string]any{
		"messages": messages,
	})
}

// Sessions returns all session summaries from the host.
func (e *Extension) Sessions(ctx context.Context) ([]SessionInfo, error) {
	type resp struct {
		Sessions []SessionInfo `json:"sessions"`
	}
	r, err := hostCall[resp](e, ctx, "host/sessions", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Sessions, nil
}

// ForkSession forks the current session and returns the parent ID and message count.
func (e *Extension) ForkSession(ctx context.Context) (parentID string, count int, err error) {
	type resp struct {
		ParentID     string `json:"parentID"`
		MessageCount int    `json:"messageCount"`
	}
	r, err := hostCall[resp](e, ctx, "host/forkSession", struct{}{})
	if err != nil {
		return "", 0, err
	}
	return r.ParentID, r.MessageCount, nil
}

// SetSessionTitle sets the current session's title.
func (e *Extension) SetSessionTitle(ctx context.Context, title string) error {
	return hostCallVoid(e, ctx, "host/setSessionTitle", map[string]any{"title": title})
}

// LoadSession opens a session by path on the host side.
// The host enqueues a swap action; the session takes effect on the next agent turn.
func (e *Extension) LoadSession(ctx context.Context, path string) error {
	return hostCallVoid(e, ctx, "host/loadSession", map[string]any{"path": path})
}

// EntryInfo is the SDK view of a session entry for display.
type EntryInfo struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentId,omitempty"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"` // RFC3339
	Children  int    `json:"children"`
}

// SessionEntryInfos returns info about entries on the current branch for display.
func (e *Extension) SessionEntryInfos(ctx context.Context) ([]EntryInfo, error) {
	type resp struct {
		Entries []EntryInfo `json:"entries"`
	}
	r, err := hostCall[resp](e, ctx, "host/sessionEntryInfos", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Entries, nil
}

// TreeNode is the SDK view of a full-tree node for DAG rendering.
type TreeNode struct {
	ID           string `json:"id"`
	ParentID     string `json:"parentId,omitempty"`
	Type         string `json:"type"`
	Timestamp    string `json:"timestamp"` // RFC3339
	Children     int    `json:"children"`
	OnActivePath bool   `json:"onActivePath"`
	Depth        int    `json:"depth"`
	Preview      string `json:"preview,omitempty"`
	Label        string `json:"label,omitempty"`
}

// SessionFullTree returns every entry in the session for DAG rendering.
func (e *Extension) SessionFullTree(ctx context.Context) ([]TreeNode, error) {
	type resp struct {
		Nodes []TreeNode `json:"nodes"`
	}
	r, err := hostCall[resp](e, ctx, "host/sessionFullTree", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Nodes, nil
}

// SessionTitle returns the current session's title (empty if unset).
func (e *Extension) SessionTitle(ctx context.Context) (string, error) {
	type resp struct {
		Title string `json:"title"`
	}
	r, err := hostCall[resp](e, ctx, "host/sessionTitle", struct{}{})
	if err != nil {
		return "", err
	}
	return r.Title, nil
}

// WriteModels regenerates models.yaml from the embedded curated list with
// the given API overrides, writes to disk, and reloads the registry.
func (e *Extension) WriteModels(ctx context.Context, overrides map[string]ModelOverride) (int, error) {
	type resp struct {
		ModelsWritten int `json:"modelsWritten"`
	}
	r, err := hostCall[resp](e, ctx, "host/writeModels", map[string]any{"overrides": overrides})
	if err != nil {
		return 0, err
	}
	return r.ModelsWritten, nil
}

// LastAssistantText returns the text content of the last assistant message.
// Returns empty string if no assistant messages exist.
func (e *Extension) LastAssistantText(ctx context.Context) (string, error) {
	type resp struct {
		Text string `json:"text"`
	}
	r, err := hostCall[resp](e, ctx, "host/lastAssistantText", struct{}{})
	if err != nil {
		return "", err
	}
	return r.Text, nil
}

// AppendSessionEntry writes a custom extension entry to the current session.
// The kind should be namespaced (e.g., "ext:memory:facts").
func (e *Extension) AppendSessionEntry(ctx context.Context, kind string, data any) error {
	return hostCallVoid(e, ctx, "host/appendSessionEntry", map[string]any{
		"kind": kind,
		"data": data,
	})
}

// AppendCustomMessage writes a message that persists AND appears in Messages() on reload.
// Role must be "user" or "assistant". Use for durable context annotations.
func (e *Extension) AppendCustomMessage(ctx context.Context, role, content string) error {
	return hostCallVoid(e, ctx, "host/appendCustomMessage", map[string]any{
		"role":    role,
		"content": content,
	})
}

// SetLabel sets or clears a bookmark label on a session entry.
// An empty label clears the bookmark.
func (e *Extension) SetLabel(ctx context.Context, targetID, label string) error {
	return hostCallVoid(e, ctx, "host/setLabel", map[string]any{
		"targetId": targetID,
		"label":    label,
	})
}

// BranchSession moves the current session's leaf to an earlier entry.
func (e *Extension) BranchSession(ctx context.Context, entryID string) error {
	return hostCallVoid(e, ctx, "host/branchSession", map[string]any{"entryId": entryID})
}

// BranchSessionWithSummary moves the leaf and writes a branch_summary entry.
func (e *Extension) BranchSessionWithSummary(ctx context.Context, entryID, summary string) error {
	return hostCallVoid(e, ctx, "host/branchSessionWithSummary", map[string]any{
		"entryId": entryID,
		"summary": summary,
	})
}
