package usage

import (
	"encoding/json"
	"sync"
)

// TokenUsage reports token consumption with cache breakdown.
type TokenUsage struct {
	Input         int `json:"input"`
	Output        int `json:"output"`
	CacheRead     int `json:"cache_read"`
	CacheWrite    int `json:"cache_write"`
	CacheCreation int `json:"cache_creation"`
}

// ComponentBreakdown shows token usage per prompt component.
type ComponentBreakdown struct {
	SystemPrompt int                `json:"system_prompt"`
	Extensions   []ExtensionTokens  `json:"extensions"`
	RepoMap      int                `json:"repo_map"`
	Tools        int                `json:"tools"`
	History      int                `json:"history"`
}

// ExtensionTokens is token usage for a single extension's prompt section.
type ExtensionTokens struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

// TurnUsageEvent is the payload for EventTurnUsage.
type TurnUsageEvent struct {
	Turn      int                `json:"turn"`
	Usage     TokenUsage         `json:"usage"`
	Breakdown ComponentBreakdown `json:"breakdown"`
}

// SessionStats accumulates usage across a session.
type SessionStats struct {
	mu      sync.RWMutex
	turns   []TurnUsageEvent
	totals  TokenUsage
	current ComponentBreakdown
}

// NewSessionStats creates an empty stats accumulator.
func NewSessionStats() *SessionStats {
	return &SessionStats{}
}

// Record adds a turn's usage to the session totals.
func (s *SessionStats) Record(event TurnUsageEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.turns = append(s.turns, event)
	s.totals.Input += event.Usage.Input
	s.totals.Output += event.Usage.Output
	s.totals.CacheRead += event.Usage.CacheRead
	s.totals.CacheWrite += event.Usage.CacheWrite
	s.totals.CacheCreation += event.Usage.CacheCreation
	s.current = event.Breakdown
}

// Totals returns cumulative token usage.
func (s *SessionStats) Totals() TokenUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totals
}

// CurrentBreakdown returns the latest prompt component breakdown.
func (s *SessionStats) CurrentBreakdown() ComponentBreakdown {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// TurnCount returns the number of turns recorded.
func (s *SessionStats) TurnCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.turns)
}

// ParseEvent decodes a TurnUsageEvent from JSON.
func ParseEvent(data json.RawMessage) (TurnUsageEvent, error) {
	var event TurnUsageEvent
	err := json.Unmarshal(data, &event)
	return event, err
}

// FormatSummary returns a human-readable summary string.
func (s *SessionStats) FormatSummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b []byte
	b = append(b, "Session Token Usage\n"...)
	b = append(b, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"...)

	// Totals
	b = append(b, "CUMULATIVE TOTALS\n"...)
	b = append(b, formatLine("Input", s.totals.Input)...)
	b = append(b, formatLine("Output", s.totals.Output)...)
	if s.totals.CacheRead > 0 {
		b = append(b, formatLine("Cache Read", s.totals.CacheRead)...)
	}
	if s.totals.CacheWrite > 0 {
		b = append(b, formatLine("Cache Write", s.totals.CacheWrite)...)
	}
	if s.totals.CacheCreation > 0 {
		b = append(b, formatLine("Cache Creation", s.totals.CacheCreation)...)
	}
	b = append(b, "\n"...)

	// Current prompt breakdown
	b = append(b, "CURRENT PROMPT BREAKDOWN\n"...)
	b = append(b, formatLine("System Prompt", s.current.SystemPrompt)...)
	b = append(b, formatLine("Repo Map", s.current.RepoMap)...)
	b = append(b, formatLine("Tool Definitions", s.current.Tools)...)
	b = append(b, formatLine("Conversation History", s.current.History)...)

	if len(s.current.Extensions) > 0 {
		b = append(b, "\nEXTENSION PROMPT SECTIONS\n"...)
		for _, ext := range s.current.Extensions {
			b = append(b, formatLine(ext.Name, ext.Tokens)...)
		}
	}

	b = append(b, "\n"...)
	b = append(b, formatLine("Total Turns", len(s.turns))...)

	return string(b)
}

func formatLine(label string, value int) []byte {
	return []byte(formatRightAlign(label, value) + "\n")
}

func formatRightAlign(label string, value int) string {
	// Label left, number right-aligned in 12-char field
	num := formatNumber(value)
	padding := 40 - len(label) - len(num)
	if padding < 1 {
		padding = 1
	}
	return label + repeat(" ", padding) + num
}

func formatNumber(n int) string {
	if n >= 1000000 {
		return formatWithCommas(n)
	}
	if n >= 1000 {
		return formatWithCommas(n)
	}
	return intToStr(n)
}

func formatWithCommas(n int) string {
	s := intToStr(n)
	result := s
	if len(s) > 3 {
		result = s[:len(s)-3] + "," + s[len(s)-3:]
	}
	return result
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var neg bool
	if n < 0 {
		neg = true
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func repeat(s string, n int) string {
	result := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		result = append(result, s...)
	}
	return string(result)
}
