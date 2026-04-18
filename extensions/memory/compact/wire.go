package compact

import "encoding/json"

// WireMsg wraps a message with a type discriminator for JSON transport.
// Matches the host's CompactMessage wire format used by ConversationMessages.
// Defined here so both compact/ and memory/clearer.go can share it via type alias,
// avoiding an import cycle (memory/ imports compact/, compact/ does not import memory/).
type WireMsg struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// WireToolResult is the wire representation of a ToolResultMessage.
type WireToolResult struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}
