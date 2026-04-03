// Package core defines the domain types for piglet: messages, content, events, tools.
package core

import (
	"context"
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------------------
// Content types
// ---------------------------------------------------------------------------

// ContentBlock is content in user messages and tool results.
// Implementations: TextContent, ImageContent.
type ContentBlock interface{ isContentBlock() }

// AssistantContent is content in assistant messages.
// Implementations: TextContent, ThinkingContent, ToolCall.
type AssistantContent interface{ isAssistantContent() }

// TextContent is a text block.
type TextContent struct {
	Text string `json:"text"`
}

func (TextContent) isContentBlock()     {}
func (TextContent) isAssistantContent() {}

// ThinkingContent is a reasoning/thinking block.
type ThinkingContent struct {
	Thinking string `json:"thinking"`
}

func (ThinkingContent) isAssistantContent() {}

// ImageContent is a base64-encoded image.
type ImageContent struct {
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

func (ImageContent) isContentBlock() {}

// ToolCall is a request from the assistant to invoke a tool.
type ToolCall struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Arguments    map[string]any `json:"arguments"`
	ProviderMeta map[string]any `json:"providerMeta,omitempty"` // opaque provider data (e.g. Gemini thought signatures)
}

func (ToolCall) isAssistantContent() {}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// Message is the sealed union of conversation messages.
// Use a type switch: *UserMessage, *AssistantMessage, *ToolResultMessage.
type Message interface{ isMessage() }

// UserMessage is a message from the user.
type UserMessage struct {
	Content   string         `json:"content,omitempty"`
	Blocks    []ContentBlock `json:"blocks,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

func (*UserMessage) isMessage() {}

// UnmarshalJSON implements custom unmarshaling for UserMessage
// because Blocks is an interface slice ([]ContentBlock).
func (m *UserMessage) UnmarshalJSON(data []byte) error {
	type Alias UserMessage
	raw := struct {
		Alias
		Blocks []json.RawMessage `json:"blocks"`
	}{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = UserMessage(raw.Alias)
	m.Blocks = unmarshalContentBlocks(raw.Blocks)
	return nil
}

// AssistantMessage is a response from the LLM.
type AssistantMessage struct {
	Content    []AssistantContent `json:"content"`
	Model      string             `json:"model"`
	Provider   string             `json:"provider"`
	Usage      Usage              `json:"usage"`
	StopReason StopReason         `json:"stopReason"`
	Error      string             `json:"error,omitempty"`
	Timestamp  time.Time          `json:"timestamp"`
}

func (*AssistantMessage) isMessage() {}

// ToolResultMessage is the result of a tool execution sent back to the LLM.
type ToolResultMessage struct {
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Content    []ContentBlock `json:"content"`
	IsError    bool           `json:"isError"`
	Timestamp  time.Time      `json:"timestamp"`
}

func (*ToolResultMessage) isMessage() {}

func unmarshalContentBlocks(rawBlocks []json.RawMessage) []ContentBlock {
	var blocks []ContentBlock
	for _, r := range rawBlocks {
		var probe struct {
			MimeType string `json:"mimeType"`
		}
		_ = json.Unmarshal(r, &probe)

		if probe.MimeType != "" {
			var ic ImageContent
			if err := json.Unmarshal(r, &ic); err == nil {
				blocks = append(blocks, ic)
			}
		} else {
			var tc TextContent
			if err := json.Unmarshal(r, &tc); err == nil {
				blocks = append(blocks, tc)
			}
		}
	}
	return blocks
}

// UnmarshalJSON implements custom unmarshaling for AssistantMessage
// because Content is an interface slice ([]AssistantContent).
func (m *AssistantMessage) UnmarshalJSON(data []byte) error {
	type Alias AssistantMessage
	raw := struct {
		Alias
		Content []json.RawMessage `json:"content"`
	}{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = AssistantMessage(raw.Alias)
	m.Content = nil

	for _, r := range raw.Content {
		var probe struct {
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
			ID       string `json:"id"`
			Name     string `json:"name"`
		}
		_ = json.Unmarshal(r, &probe)

		switch {
		case probe.ID != "" || probe.Name != "":
			var tc ToolCall
			if err := json.Unmarshal(r, &tc); err == nil {
				m.Content = append(m.Content, tc)
			}
		case probe.Thinking != "":
			var tc ThinkingContent
			if err := json.Unmarshal(r, &tc); err == nil {
				m.Content = append(m.Content, tc)
			}
		default:
			var tc TextContent
			if err := json.Unmarshal(r, &tc); err == nil {
				m.Content = append(m.Content, tc)
			}
		}
	}
	return nil
}

// UnmarshalJSON implements custom unmarshaling for ToolResultMessage
// because Content is an interface slice ([]ContentBlock).
func (m *ToolResultMessage) UnmarshalJSON(data []byte) error {
	type Alias ToolResultMessage
	raw := struct {
		Alias
		Content []json.RawMessage `json:"content"`
	}{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = ToolResultMessage(raw.Alias)
	m.Content = unmarshalContentBlocks(raw.Content)
	return nil
}

// StopReason indicates why the assistant stopped generating.
type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonTool    StopReason = "toolUse"
	StopReasonError   StopReason = "error"
	StopReasonAborted StopReason = "aborted"
)

// Usage tracks token consumption and cost for an LLM call.
type Usage struct {
	InputTokens      int     `json:"inputTokens"`
	OutputTokens     int     `json:"outputTokens"`
	CacheReadTokens  int     `json:"cacheReadTokens"`
	CacheWriteTokens int     `json:"cacheWriteTokens"`
	Cost             float64 `json:"cost"`
}

// ---------------------------------------------------------------------------
// Tools
// ---------------------------------------------------------------------------

// ToolSchema is the definition sent to the LLM so it knows what tools exist.
type ToolSchema struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema object
}

// ToolResult is what a tool returns after execution.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	Details any            `json:"details,omitempty"` // opaque, forwarded to UI via events
}

// Tool is a schema plus its execute function.
type Tool struct {
	ToolSchema
	Execute ToolExecuteFn
}

// ToolExecuteFn is the function signature for tool execution.
// ctx carries cancellation (e.g., from steering or abort).
// id is the tool call ID from the LLM. args are the parsed JSON arguments.
type ToolExecuteFn func(ctx context.Context, id string, args map[string]any) (*ToolResult, error)

// ---------------------------------------------------------------------------
// Events (agent → consumer)
// ---------------------------------------------------------------------------

// Event is emitted by the agent on a buffered channel.
// Use a type switch on the concrete type.
type Event interface{ eventType() string }

// EventAgentStart signals the agent loop has begun.
type EventAgentStart struct{}

func (EventAgentStart) eventType() string { return "agent_start" }

// EventAgentEnd signals the agent loop has finished.
type EventAgentEnd struct {
	Messages []Message
}

func (EventAgentEnd) eventType() string { return "agent_end" }

// EventTurnStart signals a new turn (LLM call + tool execution).
type EventTurnStart struct{}

func (EventTurnStart) eventType() string { return "turn_start" }

// EventTurnEnd signals a turn has completed.
type EventTurnEnd struct {
	Assistant   *AssistantMessage
	ToolResults []*ToolResultMessage
}

func (EventTurnEnd) eventType() string { return "turn_end" }

// EventStreamDelta carries an incremental streaming update.
type EventStreamDelta struct {
	Kind  string // "text", "thinking", "toolcall"
	Index int
	Delta string
}

func (EventStreamDelta) eventType() string { return "stream_delta" }

// EventStreamDone signals the LLM finished streaming. Message is complete.
type EventStreamDone struct {
	Message *AssistantMessage
}

func (EventStreamDone) eventType() string { return "stream_done" }

// EventToolStart signals a tool execution is beginning.
type EventToolStart struct {
	ToolCallID string
	ToolName   string
	Args       map[string]any
}

func (EventToolStart) eventType() string { return "tool_start" }

// EventToolUpdate carries a partial tool result for live display.
type EventToolUpdate struct {
	ToolCallID string
	ToolName   string
	Partial    any
}

func (EventToolUpdate) eventType() string { return "tool_update" }

// EventToolEnd signals a tool execution has finished.
type EventToolEnd struct {
	ToolCallID string
	ToolName   string
	Result     any
	IsError    bool
}

func (EventToolEnd) eventType() string { return "tool_end" }

// EventRetry signals the agent is retrying after a transient error.
type EventRetry struct {
	Attempt int
	Max     int
	DelayMs int
	Error   string
}

func (EventRetry) eventType() string { return "retry" }

// EventMaxTurns signals the agent stopped because MaxTurns was reached.
type EventMaxTurns struct {
	Count int
	Max   int
}

func (EventMaxTurns) eventType() string { return "max_turns" }

// EventStepWait signals the agent is paused, waiting for step approval.
type EventStepWait struct {
	ToolCallID string
	ToolName   string
	Args       map[string]any
}

func (EventStepWait) eventType() string { return "step_wait" }

// EventCompact signals that auto-compaction occurred.
type EventCompact struct {
	Before          int
	After           int
	TokensAtCompact int
}

func (EventCompact) eventType() string { return "compact" }

// EventSessionLoad signals that the agent has pre-loaded conversation history.
// Emitted at the start of a run when messages were restored from a prior session.
type EventSessionLoad struct {
	MessageCount int
}

func (EventSessionLoad) eventType() string { return "session_load" }

// EventAgentInit signals the agent is fully configured and about to process.
// Emitted after session load check, before the first user message is appended.
type EventAgentInit struct {
	ToolCount int
}

func (EventAgentInit) eventType() string { return "agent_init" }

// EventPromptBuild signals the final assembled system prompt.
// Extensions can observe it for debugging or logging.
type EventPromptBuild struct {
	System string
}

func (EventPromptBuild) eventType() string { return "prompt_build" }

// EventMessagePre signals a user message is about to be appended to history.
// Emitted before the message enters the conversation, allowing observers to react.
type EventMessagePre struct {
	Content string
}

func (EventMessagePre) eventType() string { return "message_pre" }
