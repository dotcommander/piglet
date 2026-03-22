package autotitle

import (
	"context"
	"strings"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
)

// MaxTitleRunes caps generated titles.
const MaxTitleRunes = 50

// LoadPrompt reads the title generation system prompt from ~/.config/piglet/autotitle.md.
// Returns empty string if the file doesn't exist.
func LoadPrompt() string {
	s, _ := config.ReadExtensionConfig("autotitle")
	return s
}

// GenerateTitle produces a short session title from the first user/assistant exchange.
// Uses a lightweight streaming LLM call with a 10-second timeout.
// Returns empty string on failure (best-effort).
func GenerateTitle(ctx context.Context, prov core.StreamProvider, messages []core.Message, systemPrompt string) string {
	if prov == nil || systemPrompt == "" {
		return ""
	}

	userText, assistantText := extractFirstExchange(messages)
	if userText == "" {
		return ""
	}

	// Truncate to keep the title prompt small
	userText = truncateRunes(userText, 200)
	assistantText = truncateRunes(assistantText, 200)

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	maxTok := 30
	ch := prov.Stream(callCtx, core.StreamRequest{
		System: systemPrompt,
		Messages: []core.Message{
			&core.UserMessage{Content: "User: " + userText + "\n\nAssistant: " + assistantText},
		},
		Options: core.StreamOptions{MaxTokens: &maxTok},
	})

	var title strings.Builder
	for evt := range ch {
		if evt.Type == core.StreamTextDelta {
			title.WriteString(evt.Delta)
		}
	}

	result := strings.TrimSpace(title.String())
	if result == "" {
		return ""
	}
	return truncateRunes(result, MaxTitleRunes)
}

func extractFirstExchange(messages []core.Message) (userText, assistantText string) {
	for _, msg := range messages {
		switch v := msg.(type) {
		case *core.UserMessage:
			if userText == "" {
				userText = v.Content
			}
		case *core.AssistantMessage:
			if assistantText == "" {
				for _, c := range v.Content {
					if tc, ok := c.(core.TextContent); ok {
						assistantText = tc.Text
						break
					}
				}
			}
		}
		if userText != "" && assistantText != "" {
			break
		}
	}
	return
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return s
}
