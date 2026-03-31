package core

import "context"

// ---------------------------------------------------------------------------
// Provider interface
// ---------------------------------------------------------------------------

// StreamProvider is the single interface all LLM providers implement.
// Three implementations: OpenAI-compatible, Anthropic, Google.
//
// Stream returns a channel of StreamEvents. Implementations MUST close the
// channel when ctx is cancelled or streaming completes. Failure to close the
// channel leaks the consumer goroutine.
type StreamProvider interface {
	Stream(ctx context.Context, req StreamRequest) <-chan StreamEvent
}

// StreamRequest is the input to a provider's Stream method.
type StreamRequest struct {
	System   string
	Messages []Message
	Tools    []ToolSchema
	Options  StreamOptions
}

// StreamOptions configures an LLM streaming call.
type StreamOptions struct {
	Temperature *float64
	MaxTokens   *int
	Thinking    ThinkingLevel
	APIKeyFunc  func(provider string) string
	Headers     map[string]string
}

// StreamEvent is emitted by providers during streaming.
// Type discriminates; optional fields carry data for that type.
type StreamEvent struct {
	Type    string            // "text_delta", "thinking_delta", "toolcall_delta", "toolcall_end", "done", "error"
	Index   int               // content block index
	Delta   string            // incremental text for delta events
	Tool    *ToolCall         // complete tool call on "toolcall_end"
	Message *AssistantMessage // complete message on "done" or "error"
	Error   error             // set on "error"
}

// Stream event type constants.
const (
	StreamTextDelta     = "text_delta"
	StreamThinkingDelta = "thinking_delta"
	StreamToolCallDelta = "toolcall_delta"
	StreamToolCallEnd   = "toolcall_end"
	StreamDone          = "done"
	StreamError         = "error"
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// API identifies the wire protocol for a model.
type API string

const (
	APIOpenAI    API = "openai"
	APIAnthropic API = "anthropic"
	APIGoogle    API = "google"
)

// ThinkingLevel controls reasoning depth for models that support it.
type ThinkingLevel string

const (
	ThinkingOff    ThinkingLevel = "off"
	ThinkingLow    ThinkingLevel = "low"
	ThinkingMedium ThinkingLevel = "medium"
	ThinkingHigh   ThinkingLevel = "high"
)

// Model describes an LLM endpoint.
type Model struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	API           API               `json:"api"`
	Provider      string            `json:"provider"`
	BaseURL       string            `json:"baseUrl,omitempty"`
	Reasoning     bool              `json:"reasoning"`
	ContextWindow int               `json:"contextWindow"`
	MaxTokens     int               `json:"maxTokens"`
	Cost          ModelCost         `json:"cost"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// DisplayName returns "provider/name" for UI display.
func (m Model) DisplayName() string {
	return m.Provider + "/" + m.Name
}

// ModelCost is per-million-token pricing.
type ModelCost struct {
	Input      float64 `json:"input" yaml:"input"`
	Output     float64 `json:"output" yaml:"output"`
	CacheRead  float64 `json:"cacheRead" yaml:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite" yaml:"cacheWrite"`
}
