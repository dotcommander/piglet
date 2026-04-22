package external

import "encoding/json"

// ---------------------------------------------------------------------------
// Host session service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostConversationMessagesResult is the host's response with raw message JSON.
type HostConversationMessagesResult struct {
	Messages json.RawMessage `json:"messages"`
}

// HostSetConversationMessagesParams is the extension's request to replace messages.
type HostSetConversationMessagesParams struct {
	Messages []CompactMessage `json:"messages"`
}

// HostSessionsResult is the host's response with session summaries.
type HostSessionsResult struct {
	Sessions []WireSessionInfo `json:"sessions"`
}

// WireSessionInfo is the wire representation of a session summary.
type WireSessionInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CWD       string `json:"cwd"`
	CreatedAt string `json:"createdAt"` // RFC3339
	ParentID  string `json:"parentId,omitempty"`
	Path      string `json:"path"`
}

// HostLoadSessionParams requests loading a session by path.
type HostLoadSessionParams struct {
	Path string `json:"path"`
}

// HostForkSessionResult is the host's response after forking.
type HostForkSessionResult struct {
	ParentID     string `json:"parentID"`
	MessageCount int    `json:"messageCount"`
}

// HostSetSessionTitleParams sets the current session's title.
type HostSetSessionTitleParams struct {
	Title string `json:"title"`
}

// HostSyncModelsResult is the host's response after syncing models.
type HostSyncModelsResult struct {
	Updated int `json:"updated"`
}

// HostWriteModelsParams holds API-sourced overrides for regenerating models.yaml.
type HostWriteModelsParams struct {
	Overrides map[string]HostModelOverride `json:"overrides"`
}

// HostModelOverride holds values that replace curated defaults for one model.
type HostModelOverride struct {
	Name          string `json:"name,omitempty"`
	ContextWindow int    `json:"contextWindow,omitempty"`
	MaxTokens     int    `json:"maxTokens,omitempty"`
}

// HostWriteModelsResult is the host's response after writing models.
type HostWriteModelsResult struct {
	ModelsWritten int `json:"modelsWritten"`
}

// HostRunBackgroundParams starts a background agent.
type HostRunBackgroundParams struct {
	Prompt string `json:"prompt"`
}

// HostIsBackgroundRunningResult is the host's response.
type HostIsBackgroundRunningResult struct {
	Running bool `json:"running"`
}

// ---------------------------------------------------------------------------
// Host session entry service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostAppendSessionEntryParams appends a custom entry to the current session.
type HostAppendSessionEntryParams struct {
	Kind string `json:"kind"` // namespaced, e.g. "ext:memory:facts"
	Data any    `json:"data"`
}

// HostAppendCustomMessageParams writes a custom message to the session.
type HostAppendCustomMessageParams struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// HostSetLabelParams sets or clears a bookmark label on a session entry.
type HostSetLabelParams struct {
	TargetID string `json:"targetId"`
	Label    string `json:"label"`
}

// HostBranchSessionParams branches the session at a specific entry.
type HostBranchSessionParams struct {
	EntryID string `json:"entryId"`
}

// HostBranchSessionSummaryParams branches with a summary annotation.
type HostBranchSessionSummaryParams struct {
	EntryID string `json:"entryId"`
	Summary string `json:"summary"`
}

// ---------------------------------------------------------------------------
// Host event bus service: extension → host (request/response)
// ---------------------------------------------------------------------------

// HostActivateToolParams promotes a deferred tool to full schema.
type HostActivateToolParams struct {
	Name string `json:"name"`
}

// HostSetToolFilterParams sets a per-turn tool filter by name allowlist.
type HostSetToolFilterParams struct {
	Names []string `json:"names"` // empty = clear filter (include all)
}

// HostActivateToolResult is the host's response after activating a tool.
type HostActivateToolResult struct{}

// HostPublishParams publishes data to the inter-extension event bus.
type HostPublishParams struct {
	Topic string `json:"topic"`
	Data  any    `json:"data"`
}

// HostSubscribeParams subscribes to an event bus topic.
type HostSubscribeParams struct {
	Topic string `json:"topic"`
}

// HostSubscribeResult returns a subscription ID for unsubscribing.
type HostSubscribeResult struct {
	SubscriptionID int `json:"subscriptionId"`
}

// EventBusEventParams is a host → extension notification when subscribed topics fire.
type EventBusEventParams struct {
	Topic          string          `json:"topic"`
	SubscriptionID int             `json:"subscriptionId"`
	Data           json.RawMessage `json:"data"`
}

// ---------------------------------------------------------------------------
// Host session display service: extension → host (request/response)
// Added for T5a — supports session-command extraction (T5b).
// ---------------------------------------------------------------------------

// WireEntryInfo is the wire representation of ext.EntryInfo.
// Timestamp serialized as RFC3339 to match WireSessionInfo convention.
type WireEntryInfo struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentId,omitempty"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"` // RFC3339
	Children  int    `json:"children"`
}

// HostSessionEntryInfosResult is the host's response to host/sessionEntryInfos.
type HostSessionEntryInfosResult struct {
	Entries []WireEntryInfo `json:"entries"`
}

// WireTreeNode is the wire representation of ext.TreeNode.
type WireTreeNode struct {
	ID           string `json:"id"`
	ParentID     string `json:"parentId,omitempty"`
	Type         string `json:"type"`
	Timestamp    string `json:"timestamp"` // RFC3339
	Children     int    `json:"children"`
	OnActivePath bool   `json:"onActivePath"`
	Depth        int    `json:"depth"`
	Preview      string `json:"preview,omitempty"`
	Label        string `json:"label,omitempty"`
	TokensBefore int    `json:"tokensBefore,omitempty"` // tokens before compaction; only set on compact entries
}

// HostSessionFullTreeResult is the host's response to host/sessionFullTree.
type HostSessionFullTreeResult struct {
	Nodes []WireTreeNode `json:"nodes"`
}

// HostSessionTitleResult is the host's response to host/sessionTitle.
type HostSessionTitleResult struct {
	Title string `json:"title"`
}

// WirePickerItem mirrors ext.PickerItem.
type WirePickerItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Desc  string `json:"desc,omitempty"`
}

// HostShowPickerParams is the extension's request to display a picker.
type HostShowPickerParams struct {
	Title string           `json:"title"`
	Items []WirePickerItem `json:"items"`
}

// HostShowPickerResult is the host's response after the user selects an item.
// Selected is the ID of the chosen item. If the user dismisses the picker,
// the RPC call blocks until hostRequestTimeout (the OnSelect callback in
// ActionShowPicker is never invoked for dismissal in the current TUI).
type HostShowPickerResult struct {
	Selected string `json:"selected"`
}
