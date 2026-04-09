package provider

// ---------------------------------------------------------------------------
// OpenAI wire types — request/response DTOs for the chat completions API.
// ---------------------------------------------------------------------------

type oaiRequest struct {
	Model               string            `json:"model"`
	Messages            []oaiMessage      `json:"messages"`
	MaxTokens           *int              `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	Temperature         *float64          `json:"temperature,omitempty"`
	Stream              bool              `json:"stream"`
	Tools               []oaiTool         `json:"tools,omitempty"`
	ToolChoice          any               `json:"tool_choice,omitempty"`
	StreamOptions       *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type oaiToolCall struct {
	Index    *int            `json:"index,omitempty"`
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiFunctionCall `json:"function"`
}

type oaiFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

// ---------------------------------------------------------------------------
// OpenAI streaming event wire types.
// ---------------------------------------------------------------------------

type oaiStreamEvent struct {
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage,omitempty"`
}

type oaiChoice struct {
	Index        int             `json:"index"`
	Delta        *oaiChoiceDelta `json:"delta"`
	FinishReason string          `json:"finish_reason"`
}

type oaiChoiceDelta struct {
	Content   string        `json:"content,omitempty"`
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
}

type oaiUsage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *oaiPromptTokensInfo `json:"prompt_tokens_details,omitempty"`
}

type oaiPromptTokensInfo struct {
	CachedTokens int `json:"cached_tokens"`
}
