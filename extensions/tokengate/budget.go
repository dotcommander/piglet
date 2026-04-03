package tokengate

import (
	"encoding/json"
	"fmt"
	"sync"
)

// TurnUsageEvent matches the usage extension's event structure.
type TurnUsageEvent struct {
	Turn      int        `json:"turn"`
	Usage     TokenUsage `json:"usage"`
	Breakdown Breakdown  `json:"breakdown"`
}

type TokenUsage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
}

type Breakdown struct {
	SystemPrompt int          `json:"system_prompt"`
	Extensions   []ExtTokens  `json:"extensions"`
	RepoMap      int          `json:"repo_map"`
	Tools        int          `json:"tools"`
	History      int          `json:"history"`
}

type ExtTokens struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

// BudgetState tracks token usage against configured limits.
type BudgetState struct {
	mu          sync.RWMutex
	cfg         Config
	current     Breakdown
	totalInput  int
	totalOutput int
	turns       int
	warned      bool
}

func NewBudgetState(cfg Config) *BudgetState {
	return &BudgetState{cfg: cfg}
}

// Record updates budget state from a turn usage event.
// Returns true if the warning threshold was just crossed (first time only).
func (b *BudgetState) Record(event TurnUsageEvent) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.current = event.Breakdown
	b.totalInput += event.Usage.Input
	b.totalOutput += event.Usage.Output
	b.turns++

	if !b.warned && b.cfg.ContextWindow > 0 && b.cfg.WarnPercent > 0 {
		used := b.promptTokens()
		threshold := b.cfg.ContextWindow * b.cfg.WarnPercent / 100
		if used >= threshold {
			b.warned = true
			return true
		}
	}
	return false
}

// promptTokens returns current prompt fill (everything that counts toward context window).
func (b *BudgetState) promptTokens() int {
	total := b.current.SystemPrompt + b.current.RepoMap + b.current.Tools + b.current.History
	for _, ext := range b.current.Extensions {
		total += ext.Tokens
	}
	return total
}

// Summary returns a formatted budget summary string.
func (b *BudgetState) Summary() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	used := b.promptTokens()
	pct := 0
	if b.cfg.ContextWindow > 0 {
		pct = used * 100 / b.cfg.ContextWindow
	}

	var buf []byte
	buf = append(buf, "Context Budget\n"...)
	buf = append(buf, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"...)

	buf = append(buf, fmt.Sprintf("Context window:    %s tokens\n", fmtNum(b.cfg.ContextWindow))...)
	buf = append(buf, fmt.Sprintf("Current fill:      %s tokens (%d%%)\n", fmtNum(used), pct)...)
	buf = append(buf, fmt.Sprintf("Remaining:         %s tokens\n\n", fmtNum(b.cfg.ContextWindow-used))...)

	buf = append(buf, "BREAKDOWN\n"...)
	buf = append(buf, fmt.Sprintf("  System prompt:     %s\n", fmtNum(b.current.SystemPrompt))...)
	buf = append(buf, fmt.Sprintf("  Repo map:          %s\n", fmtNum(b.current.RepoMap))...)
	buf = append(buf, fmt.Sprintf("  Tool definitions:  %s\n", fmtNum(b.current.Tools))...)
	buf = append(buf, fmt.Sprintf("  Conversation:      %s\n", fmtNum(b.current.History))...)

	if len(b.current.Extensions) > 0 {
		buf = append(buf, "\n  Extensions:\n"...)
		for _, ext := range b.current.Extensions {
			buf = append(buf, fmt.Sprintf("    %-18s %s\n", ext.Name, fmtNum(ext.Tokens))...)
		}
	}

	buf = append(buf, fmt.Sprintf("\nCUMULATIVE\n")...)
	buf = append(buf, fmt.Sprintf("  Turns:             %d\n", b.turns)...)
	buf = append(buf, fmt.Sprintf("  Total input:       %s\n", fmtNum(b.totalInput))...)
	buf = append(buf, fmt.Sprintf("  Total output:      %s\n", fmtNum(b.totalOutput))...)

	return string(buf)
}

// ParseTurnUsage decodes a TurnUsageEvent from JSON.
func ParseTurnUsage(data json.RawMessage) (TurnUsageEvent, error) {
	var event TurnUsageEvent
	err := json.Unmarshal(data, &event)
	return event, err
}

func fmtNum(n int) string {
	if n < 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	// Insert commas
	result := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
