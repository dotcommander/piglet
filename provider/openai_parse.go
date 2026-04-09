package provider

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/dotcommander/piglet/core"
)

// ---------------------------------------------------------------------------
// SSE Stream parsing
// ---------------------------------------------------------------------------

func (p *OpenAI) ParseResponse(ctx context.Context, reader io.Reader, ch chan<- core.StreamEvent) core.AssistantMessage {
	var msg core.AssistantMessage
	toolArgs := make(map[int]*strings.Builder)
	textBuilders := make(map[int]*strings.Builder)

	ScanSSE(ctx, reader, ch, func(data []byte) {
		var evt oaiStreamEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return
		}

		// Usage
		if evt.Usage != nil {
			msg.Usage = core.Usage{
				InputTokens:  evt.Usage.PromptTokens,
				OutputTokens: evt.Usage.CompletionTokens,
			}
			if evt.Usage.PromptTokensDetails != nil {
				msg.Usage.CacheReadTokens = evt.Usage.PromptTokensDetails.CachedTokens
			}
		}

		if len(evt.Choices) == 0 {
			return
		}

		choice := evt.Choices[0]

		// Text delta
		if choice.Delta != nil && choice.Delta.Content != "" {
			ch <- core.StreamEvent{Type: core.StreamTextDelta, Delta: choice.Delta.Content}
			AppendTextBuilder(&msg, choice.Delta.Content, textBuilders)
		}

		// Tool call deltas
		if choice.Delta != nil {
			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}

				ensureToolCall(&msg, idx, tc)

				if tc.Function.Arguments != "" {
					if _, ok := toolArgs[idx]; !ok {
						toolArgs[idx] = &strings.Builder{}
					}
					toolArgs[idx].WriteString(tc.Function.Arguments)

					ch <- core.StreamEvent{
						Type:  core.StreamToolCallDelta,
						Index: idx,
						Delta: tc.Function.Arguments,
					}
				}
			}
		}

		// Finish reason
		if choice.FinishReason != "" {
			msg.StopReason = mapStopReason(choice.FinishReason)
		}
	})

	FinalizeTextBuilders(&msg, textBuilders)

	// Finalize tool call arguments
	for idx, builder := range toolArgs {
		finalizeToolArgs(&msg, idx, builder.String())
	}

	return msg
}

// findToolCallAt returns the msg.Content index of the Nth ToolCall entry,
// or -1 if fewer than idx+1 tool calls exist.
func findToolCallAt(msg *core.AssistantMessage, idx int) int {
	toolIdx := 0
	for i, c := range msg.Content {
		if _, ok := c.(core.ToolCall); ok {
			if toolIdx == idx {
				return i
			}
			toolIdx++
		}
	}
	return -1
}

func ensureToolCall(msg *core.AssistantMessage, idx int, tc oaiToolCall) {
	if i := findToolCallAt(msg, idx); i >= 0 {
		existing := msg.Content[i].(core.ToolCall)
		if tc.ID != "" {
			existing.ID = tc.ID
		}
		if tc.Function.Name != "" {
			existing.Name = tc.Function.Name
		}
		msg.Content[i] = existing
		return
	}

	// Create new tool call
	msg.Content = append(msg.Content, core.ToolCall{
		ID:        tc.ID,
		Name:      tc.Function.Name,
		Arguments: map[string]any{},
	})
}

func finalizeToolArgs(msg *core.AssistantMessage, idx int, argsJSON string) {
	if i := findToolCallAt(msg, idx); i >= 0 {
		tc := msg.Content[i].(core.ToolCall)
		var args map[string]any
		if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
			tc.Arguments = args
		}
		msg.Content[i] = tc
	}
}

var oaiStopReasons = map[string]core.StopReason{
	"stop":       core.StopReasonStop,
	"length":     core.StopReasonLength,
	"tool_calls": core.StopReasonTool,
}

func mapStopReason(reason string) core.StopReason {
	return MapStopReasonFromTable(reason, oaiStopReasons)
}
