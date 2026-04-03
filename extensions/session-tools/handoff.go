package sessiontools

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/handoff-message.md
var defaultHandoffMsg string

// Handoff builds a structured summary from memory facts and forks the session.
// The summary is injected as a user message so the new session has full context.
// When cfg.SummaryMode is "llm" or "auto" (with sufficient facts), the summary
// is refined via an LLM call before injection.
func Handoff(ctx context.Context, ext *sdk.Extension, cwd, focus string, cfg Config) error {
	summary, facts, err := BuildSummary(cwd)
	if err != nil {
		return fmt.Errorf("build handoff summary: %w", err)
	}

	if shouldEnhance(facts, cfg) {
		enhanceCtx, cancel := context.WithTimeout(ctx, cfg.LLMTimeout)
		defer cancel()
		summary = EnhanceSummary(enhanceCtx, ext, summary, cfg)
	}

	var msg strings.Builder
	msg.WriteString(summary)

	if focus != "" {
		msg.WriteString("\n\n## Requested Focus\n\n")
		msg.WriteString(focus)
	}

	msg.WriteString("\n\n" + defaultHandoffContent())

	// Inject the summary so it appears as a user message in the new session
	ext.SendMessage(msg.String())

	parentID, count, err := ext.ForkSession(ctx)
	if err != nil {
		return fmt.Errorf("fork session: %w", err)
	}

	// After ForkSession we are in the new session. Look up the parent path
	// so session_query can reference it directly.
	sessions, err := ext.Sessions(ctx)
	if err == nil {
		for _, s := range sessions {
			if s.ID == parentID && s.Path != "" {
				ext.SendMessage(fmt.Sprintf("[Parent Session: %s]", s.Path))
				break
			}
		}
	}

	ext.ShowMessage(fmt.Sprintf("Handoff complete. Forked from %s (%d messages). Summary injected into new session.", parentID, count))
	return nil
}

func defaultHandoffContent() string {
	return strings.TrimSpace(defaultHandoffMsg)
}
