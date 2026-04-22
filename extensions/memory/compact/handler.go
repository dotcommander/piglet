package compact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

// compactConfig holds configurable parameters for the compaction handler.
type compactConfig struct {
	KeepRecent          int           `yaml:"keep_recent"`
	TruncateToolResult  int           `yaml:"truncate_tool_result"`
	LightTrimMaxLen     int           `yaml:"light_trim_max_len"`
	SkipLLMThreshold    int           `yaml:"skip_llm_threshold"`
	SufficientAfterTrim int           `yaml:"sufficient_after_trim"`
	Cooldown            time.Duration `yaml:"cooldown"`
}

func defaultCompactConfig() compactConfig {
	return compactConfig{
		KeepRecent:         6,
		TruncateToolResult: 2000,
		LightTrimMaxLen:    2000,
		SkipLLMThreshold:   8000,
		Cooldown:           60 * time.Second,
	}
}

// Handler owns per-compactor state — cooldown tracking — and exposes a
// single Handle method that satisfies the SDK CompactorDef.Compact signature.
// Construct via newHandler; the zero value is not valid.
type Handler struct {
	ext      *sdk.Extension
	s        Storer
	reinject ReinjectFunc
	cfg      compactConfig

	mu            sync.Mutex
	lastCompactAt time.Time
}

// newHandler constructs a Handler, loading config from compact.yaml.
func newHandler(ext *sdk.Extension, s Storer, reinject ReinjectFunc) *Handler {
	return &Handler{
		ext:      ext,
		s:        s,
		reinject: reinject,
		cfg:      xdg.LoadYAMLExt("memory", "compact.yaml", defaultCompactConfig()),
	}
}

// Handle is the SDK compact handler that works with raw JSON messages.
// It reads facts from the store, optionally refines with an LLM call, and keeps
// the last keepRecent messages prepended with a summary reference message.
//
// If a cooldown is configured and a compaction ran recently, Handle returns the
// input unchanged — a silent no-op that prevents back-to-back compactions when
// a session sits at the token threshold.
func (h *Handler) Handle(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	// Cooldown guard: skip if a compaction completed within the cooldown window.
	if h.cfg.Cooldown > 0 {
		h.mu.Lock()
		skip := !h.lastCompactAt.IsZero() && time.Since(h.lastCompactAt) < h.cfg.Cooldown
		h.mu.Unlock()
		if skip {
			return raw, nil
		}
	}

	params, err := parseCompactMessages(raw)
	if err != nil {
		return nil, err
	}
	if len(params.Messages) <= h.cfg.KeepRecent+1 {
		return raw, nil
	}

	applyLightweightPasses(params.Messages, h.cfg)

	if h.cfg.SufficientAfterTrim > 0 && estimateTokens(params.Messages) <= h.cfg.SufficientAfterTrim {
		// Lightweight trim brought tokens below threshold — skip LLM summary.
		// Still counts as a compaction attempt: reset the clock so the guard
		// prevents an immediate retry on the next turn.
		h.mu.Lock()
		h.lastCompactAt = time.Now()
		h.mu.Unlock()
		return encodeAllMessages(params.Messages)
	}

	summary := buildSummary(ctx, h.ext, params.Messages, h.s, h.reinject, h.cfg)
	result, err := buildCompactWire(params.Messages, h.s, summary, h.reinject, h.cfg.KeepRecent)

	if err == nil {
		h.mu.Lock()
		h.lastCompactAt = time.Now()
		h.mu.Unlock()
	}

	return result, err
}

// parseCompactMessages unmarshals the raw compact payload into its message slice.
func parseCompactMessages(raw json.RawMessage) (struct {
	Messages []WireMsg `json:"messages"`
}, error) {
	var params struct {
		Messages []WireMsg `json:"messages"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("unmarshal compact params: %w", err)
	}
	return params, nil
}

// applyLightweightPasses runs the three pre-summarization compaction stages:
// microcompact old tool results, light-trim long text blocks, and truncate
// remaining tool results for the summarizer's token budget.
func applyLightweightPasses(msgs []WireMsg, cfg compactConfig) {
	microcompactToolResults(msgs, cfg.KeepRecent)
	lightTrimMessages(msgs, cfg.KeepRecent, cfg.LightTrimMaxLen)
	truncateToolResults(msgs[:len(msgs)-cfg.KeepRecent], cfg.TruncateToolResult)
}

// buildSummary gathers facts, merges prior file lists, optionally refines
// with an LLM call, and persists the result to the store.
func buildSummary(ctx context.Context, ext *sdk.Extension, msgs []WireMsg, s Storer, reinject ReinjectFunc, cfg compactConfig) string {
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
func buildCompactWire(msgs []WireMsg, s Storer, summary string, reinject ReinjectFunc, keepRecent int) (json.RawMessage, error) {
	ref := buildSummaryReference(summary)
	summaryData, err := json.Marshal(map[string]any{
		"role": "user", "content": ref,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal summary message: %w", err)
	}

	kept := msgs[len(msgs)-keepRecent:]
	wire := make([]WireMsg, 0, len(kept)+2)
	wire = append(wire, WireMsg{Type: "user", Data: summaryData})

	if reinject != nil {
		reinjectMsg := reinject(s)
		if reinjectMsg != "" {
			reinjectData, err := json.Marshal(map[string]any{
				"role": "user", "content": reinjectMsg,
			})
			if err == nil {
				wire = append(wire, WireMsg{Type: "user", Data: reinjectData})
			}
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
