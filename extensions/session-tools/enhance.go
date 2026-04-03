package sessiontools

import (
	"context"
	_ "embed"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/enhance-prompt.md
var defaultEnhanceMD string

// EnhanceSummary refines a template summary via LLM.
// Returns the original unchanged if the call fails.
func EnhanceSummary(ctx context.Context, ext *sdk.Extension, templateSummary string, cfg Config) string {
	system := xdg.LoadOrCreateExt("session-handoff", "enhance-prompt.md", defaultEnhancePrompt())

	resp, err := ext.Chat(ctx, sdk.ChatRequest{
		System: system,
		Messages: []sdk.ChatMessage{
			{Role: "user", Content: templateSummary},
		},
		Model:     "small",
		MaxTokens: cfg.LLMMaxTokens,
	})
	if err != nil {
		return templateSummary
	}

	if resp.Text == "" {
		return templateSummary
	}

	return resp.Text
}

// shouldEnhance returns true if the facts warrant LLM refinement.
func shouldEnhance(facts []memoryFact, cfg Config) bool {
	switch cfg.SummaryMode {
	case SummaryModeLLM:
		return true
	case SummaryModeTemplate:
		return false
	default: // "auto"
		if len(facts) > 20 {
			return true
		}
		errorCount := 0
		for _, f := range facts {
			if strings.HasPrefix(f.Key, "ctx:error") {
				errorCount++
				if errorCount >= 3 {
					return true
				}
			}
		}
		return false
	}
}

func defaultEnhancePrompt() string {
	return strings.TrimSpace(defaultEnhanceMD)
}
