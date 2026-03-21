// Package command — LLM-powered conversation compactor extension.
package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// RegisterCompactor registers the LLM-powered auto-compactor extension.
// The compactor summarizes older messages via the streaming provider when
// token usage exceeds the threshold.
func RegisterCompactor(app *ext.App, prov core.StreamProvider, threshold int) {
	if threshold <= 0 {
		return
	}
	app.RegisterCompactor(ext.Compactor{
		Name:      "llm-summary",
		Threshold: threshold,
		Compact:   makeLLMCompactor(prov),
	})
}

func makeLLMCompactor(prov core.StreamProvider) func(context.Context, []core.Message) ([]core.Message, error) {
	return func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		// Build a text representation of the middle messages
		var b strings.Builder
		for _, m := range msgs {
			switch msg := m.(type) {
			case *core.UserMessage:
				fmt.Fprintf(&b, "User: %s\n", msg.Content)
			case *core.AssistantMessage:
				for _, c := range msg.Content {
					if tc, ok := c.(core.TextContent); ok {
						fmt.Fprintf(&b, "Assistant: %s\n", tc.Text)
					}
				}
			case *core.ToolResultMessage:
				for _, c := range msg.Content {
					if tc, ok := c.(core.TextContent); ok {
						text := tc.Text
						if len(text) > 200 {
							r := []rune(text)
							if len(r) > 200 {
								text = string(r[:200]) + "..."
							}
						}
						fmt.Fprintf(&b, "Tool(%s): %s\n", msg.ToolCallID, text)
					}
				}
			}
		}

		ch := prov.Stream(ctx, core.StreamRequest{
			System: "Summarize this conversation excerpt concisely. Preserve key decisions, file paths, errors, and outcomes. Output only the summary, no preamble.",
			Messages: []core.Message{
				&core.UserMessage{Content: b.String(), Timestamp: time.Now()},
			},
		})

		var summary strings.Builder
		for evt := range ch {
			if evt.Type == core.StreamTextDelta {
				summary.WriteString(evt.Delta)
			}
			if evt.Type == core.StreamError {
				return nil, evt.Error
			}
		}

		result := summary.String()
		if result == "" {
			return nil, fmt.Errorf("empty summary")
		}
		return core.CompactMessages(msgs, "[Conversation compacted]\n\n"+result), nil
	}
}
