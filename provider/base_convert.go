package provider

import (
	"strings"

	"github.com/dotcommander/piglet/core"
)

// ConvertToolSchemas iterates tool schemas, normalises nil parameters,
// and calls build to produce provider-specific tool definitions.
func ConvertToolSchemas[T any](tools []core.ToolSchema, build func(name, desc string, params any) T) []T {
	out := make([]T, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = map[string]any{"type": "object"}
		}
		out = append(out, build(t.Name, t.Description, params))
	}
	return out
}

// MessageConverters holds per-type callbacks for converting core messages
// to a provider-specific wire type.
type MessageConverters[T any] struct {
	User       func(*core.UserMessage) T
	Assistant  func(*core.AssistantMessage) T
	ToolResult func(*core.ToolResultMessage) T
}

// ConvertMessageList applies the appropriate converter for each message type.
func ConvertMessageList[T any](msgs []core.Message, conv MessageConverters[T]) []T {
	var out []T
	for _, m := range msgs {
		switch msg := m.(type) {
		case *core.UserMessage:
			out = append(out, conv.User(msg))
		case *core.AssistantMessage:
			out = append(out, conv.Assistant(msg))
		case *core.ToolResultMessage:
			out = append(out, conv.ToolResult(msg))
		}
	}
	return out
}

// MapStopReasonFromTable looks up a provider-specific stop reason string
// in the given table, returning core.StopReasonStop as default.
func MapStopReasonFromTable(reason string, table map[string]core.StopReason) core.StopReason {
	if r, ok := table[reason]; ok {
		return r
	}
	return core.StopReasonStop
}

// ToolResultText extracts joined text from a ToolResultMessage.
func ToolResultText(msg *core.ToolResultMessage) string {
	var parts []string
	for _, b := range msg.Content {
		if tc, ok := b.(core.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// DecodeUserBlocks converts a UserMessage's Content and Blocks into
// provider-specific typed slices using the supplied callbacks.
func DecodeUserBlocks[T any](msg *core.UserMessage, text func(string) T, image func(core.ImageContent) T) []T {
	var out []T
	if msg.Content != "" {
		out = append(out, text(msg.Content))
	}
	for _, b := range msg.Blocks {
		switch c := b.(type) {
		case core.TextContent:
			out = append(out, text(c.Text))
		case core.ImageContent:
			out = append(out, image(c))
		}
	}
	return out
}
