package tokengate

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/toolresult"
	sdk "github.com/dotcommander/piglet/sdk"
)



// registerSummarizer registers an After interceptor that auto-summarizes
// large tool results via LLM Chat.
func registerSummarizer(ext *sdk.Extension, cfg Config) {
	if !cfg.SummarizeEnabled {
		return
	}

	ext.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "tokengate-summarize",
		Priority: 20, // before memory-overflow (30) and sift (50)
		After: func(ctx context.Context, toolName string, details any) (any, error) {
			text, ok := toolresult.ExtractText(details)
			if !ok || len(text) <= cfg.SummarizeThreshold {
				return details, nil
			}

			// Don't summarize certain tools where truncation is better
			if skipSummarize(toolName) {
				return details, nil
			}

			// Truncate input to 32k chars max for the summarization call
			input := text
			if len(input) > 32768 {
				runes := []rune(input)
				input = string(runes[:32768]) + "\n...(truncated for summarization)"
			}

			resp, err := ext.Chat(ctx, sdk.ChatRequest{
				System:    LoadSummarizePrompt(),
				Messages:  []sdk.ChatMessage{{Role: "user", Content: input}},
				Model:     "small",
				MaxTokens: 2048,
			})
			if err != nil || resp.Text == "" {
				// Fallback: return original if summarization fails
				return details, nil
			}

			summary := fmt.Sprintf("[Summarized from %d chars]\n%s", len(text), resp.Text)
			return toolresult.ReplaceText(details, summary), nil
		},
	})
}

// skipSummarize returns true for tools where raw output is more useful than a summary.
func skipSummarize(toolName string) bool {
	skip := []string{
		"Read", "Edit", "Write", "MultiEdit", // file content should be exact
		"dispatch", "coordinate",              // agent results already summarized
	}
	for _, s := range skip {
		if strings.EqualFold(toolName, s) {
			return true
		}
	}
	return false
}
