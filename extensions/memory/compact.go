package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

// compactConfig holds configurable parameters for the compaction handler.
type compactConfig struct {
	KeepRecent         int `yaml:"keep_recent"`
	TruncateToolResult int `yaml:"truncate_tool_result"`
	LightTrimMaxLen    int `yaml:"light_trim_max_len"`
	SkipLLMThreshold   int `yaml:"skip_llm_threshold"`
}

func defaultCompactConfig() compactConfig {
	return compactConfig{
		KeepRecent:         6,
		TruncateToolResult: 2000,
		LightTrimMaxLen:    2000,
		SkipLLMThreshold:   8000,
	}
}

// makeCompactHandler returns the SDK compact handler that works with raw JSON messages.
// It reads facts from the store, optionally refines with an LLM call, and keeps
// the last keepRecent messages prepended with a summary reference message.
func makeCompactHandler(ext *sdk.Extension, s *Store) func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	cfg := xdg.LoadYAMLExt("memory", "compact.yaml", defaultCompactConfig())

	return func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		params, err := parseCompactMessages(raw)
		if err != nil {
			return nil, err
		}
		if len(params.Messages) <= cfg.KeepRecent+1 {
			return raw, nil
		}

		applyLightweightPasses(params.Messages, cfg)

		summary := buildSummary(ctx, ext, params.Messages, s, cfg)

		return buildCompactWire(params.Messages, s, summary, cfg.KeepRecent)
	}
}

// parseCompactMessages unmarshals the raw compact payload into its message slice.
func parseCompactMessages(raw json.RawMessage) (struct {
	Messages []wireMsg `json:"messages"`
}, error) {
	var params struct {
		Messages []wireMsg `json:"messages"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("unmarshal compact params: %w", err)
	}
	return params, nil
}

// applyLightweightPasses runs the three pre-summarization compaction stages:
// microcompact old tool results, light-trim long text blocks, and truncate
// remaining tool results for the summarizer's token budget.
func applyLightweightPasses(msgs []wireMsg, cfg compactConfig) {
	microcompactToolResults(msgs, cfg.KeepRecent)
	lightTrimMessages(msgs, cfg.KeepRecent, cfg.LightTrimMaxLen)
	truncateToolResults(msgs[:len(msgs)-cfg.KeepRecent], cfg.TruncateToolResult)
}

// buildSummary gathers facts, merges prior file lists, optionally refines
// with an LLM call, and persists the result to the store.
func buildSummary(ctx context.Context, ext *sdk.Extension, msgs []wireMsg, s *Store, cfg compactConfig) string {
	priorRead, priorModified := extractPriorFileLists(msgs)

	result := Compact(s)
	summary := result.Summary
	summary = mergeFileLists(summary, priorRead, priorModified)

	skipLLM := cfg.SkipLLMThreshold > 0 && estimateTokens(msgs) < cfg.SkipLLMThreshold

	if summary != "" && !skipLLM {
		summary = refineWithLLM(ctx, ext, summary)
	}

	WriteSummary(s, summary)
	return summary
}

// refineWithLLM asks the small model to condense the raw fact summary.
func refineWithLLM(ctx context.Context, ext *sdk.Extension, summary string) string {
	resp, err := ext.Chat(ctx, sdk.ChatRequest{
		System:   strings.TrimSpace(defaultCompactSystem),
		Messages: []sdk.ChatMessage{{Role: "user", Content: summary}},
		Model:    "small",
	})
	if err == nil && resp.Text != "" {
		return resp.Text
	}
	return summary
}

// buildCompactWire assembles the final wire payload: summary reference message,
// optional post-compact context re-injection, and the kept recent messages.
func buildCompactWire(msgs []wireMsg, s *Store, summary string, keepRecent int) (json.RawMessage, error) {
	ref := buildSummaryReference(summary)
	summaryData, err := json.Marshal(map[string]any{
		"role": "user", "content": ref,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal summary message: %w", err)
	}

	kept := msgs[len(msgs)-keepRecent:]
	wire := make([]wireMsg, 0, len(kept)+2)
	wire = append(wire, wireMsg{Type: "user", Data: summaryData})

	reinjectMsg := buildReinjectMessage(gatherCriticalContext(s))
	if reinjectMsg != "" {
		reinjectData, err := json.Marshal(map[string]any{
			"role": "user", "content": reinjectMsg,
		})
		if err == nil {
			wire = append(wire, wireMsg{Type: "user", Data: reinjectData})
		}
	}

	wire = append(wire, kept...)
	return json.Marshal(map[string]any{"messages": wire})
}

// buildSummaryReference formats the summary into a user-facing reference message.
func buildSummaryReference(summary string) string {
	var b strings.Builder
	b.WriteString("[Context compacted — session memory updated]\n\n")
	b.WriteString("Use memory_list category=_context to see accumulated context.\n")
	b.WriteString("Use memory_get to retrieve specific facts.\n")
	if summary != "" {
		b.WriteString("\nSummary: ")
		b.WriteString(summary)
	}
	return b.String()
}
