package sdk

import (
	"context"
	"encoding/json"
	"fmt"
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

// MessagesFromFork returns the conversation messages added after the given
// offset. Typical flow: call ForkSession to record the message count, do work
// that adds messages, then call MessagesFromFork(count) to retrieve only what
// has been appended since.
//
// The offset is clamped to [0, len(messages)] — out-of-range values return an
// empty array rather than an error.
func (e *Extension) MessagesFromFork(ctx context.Context, afterCount int) (json.RawMessage, error) {
	raw, err := e.ConversationMessages(ctx)
	if err != nil {
		return nil, err
	}
	var all []json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("parse messages: %w", err)
	}
	if afterCount < 0 {
		afterCount = 0
	}
	if afterCount > len(all) {
		afterCount = len(all)
	}
	tail := all[afterCount:]
	out, err := json.Marshal(tail)
	if err != nil {
		return nil, fmt.Errorf("marshal tail: %w", err)
	}
	return out, nil
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
	TokensBefore int    `json:"tokensBefore,omitempty"` // tokens before compaction; only set on compact entries (0 = absent)
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

// SessionStats aggregates usage metrics across the current conversation:
// turn count, token totals (input/output/cache), cost, and the currently-active
// model ID. Computed on-demand from AssistantMessage.Usage fields.
type SessionStats struct {
	TurnCount             int     `json:"turnCount"`
	TotalInputTokens      int     `json:"totalInputTokens"`
	TotalOutputTokens     int     `json:"totalOutputTokens"`
	TotalCacheReadTokens  int     `json:"totalCacheReadTokens"`
	TotalCacheWriteTokens int     `json:"totalCacheWriteTokens"`
	TotalCost             float64 `json:"totalCost"`
	Model                 string  `json:"model"`
}

// SessionStats returns aggregated usage metrics for the current session.
func (e *Extension) SessionStats(ctx context.Context) (SessionStats, error) {
	return hostCall[SessionStats](e, ctx, "host/sessionStats", struct{}{})
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

// WaitForIdle blocks until the agent has finished its current turn and is idle,
// or until ctx is cancelled. Returns ctx.Err() on cancellation.
func (e *Extension) WaitForIdle(ctx context.Context) error {
	return hostCallVoid(e, ctx, "host/waitForIdle", nil)
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

// ToggleStepMode toggles the agent's step-by-step mode and returns the new state.
func (e *Extension) ToggleStepMode(ctx context.Context) (bool, error) {
	type resp struct {
		On bool `json:"on"`
	}
	r, err := hostCall[resp](e, ctx, "host/toggleStepMode", struct{}{})
	if err != nil {
		return false, err
	}
	return r.On, nil
}

// RequestQuit asks the host to quit the TUI (fire-and-forget).
func (e *Extension) RequestQuit(ctx context.Context) error {
	return hostCallVoid(e, ctx, "host/requestQuit", struct{}{})
}

// HasCompactor returns true if a compactor is currently registered with the host.
func (e *Extension) HasCompactor(ctx context.Context) (bool, error) {
	type resp struct {
		Present bool `json:"present"`
	}
	r, err := hostCall[resp](e, ctx, "host/hasCompactor", struct{}{})
	if err != nil {
		return false, err
	}
	return r.Present, nil
}

// TriggerCompactResult holds the message counts before and after compaction.
type TriggerCompactResult struct {
	Before int `json:"before"`
	After  int `json:"after"`
}

// TriggerCompact asks the host to run compaction immediately using the registered
// compactor. Returns (before, after) message counts on success, or an error if
// no compactor is registered or compaction fails.
func (e *Extension) TriggerCompact(ctx context.Context) (TriggerCompactResult, error) {
	return hostCall[TriggerCompactResult](e, ctx, "host/triggerCompact", struct{}{})
}
