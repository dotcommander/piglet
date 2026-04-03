package suggest

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"

	sdk "github.com/dotcommander/piglet/sdk"
)

// TurnData represents the relevant data from an EventTurnEnd, parsed from JSON.
type TurnData struct {
	Assistant   json.RawMessage   `json:"Assistant"`
	ToolResults []ToolResult      `json:"ToolResults"`
}

// ToolResult represents a tool result from the turn.
type ToolResult struct {
	ToolName   string `json:"tool_name"`
	ToolCallID string `json:"tool_call_id"`
	IsError    bool   `json:"is_error"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// blockedPatterns are generic suggestions that should be filtered out.
var blockedPatterns = []string{
	"continue",
	"proceed",
	"done?",
	"next",
	"ok",
	"okay",
	"yes",
	"go ahead",
	"keep going",
	"what else",
	"anything else",
	"?",
	"...",
}

// Suggester generates next-prompt suggestions based on conversation context.
type Suggester struct {
	config   Config
	prompt   string
	cooldown int
	seen     map[string]struct{}
	mu       sync.Mutex
	ext      *sdk.Extension
}

// NewSuggester creates a new suggester with the given config.
func NewSuggester(cfg Config, prompt string, ext *sdk.Extension) *Suggester {
	return &Suggester{
		config:   cfg,
		prompt:   prompt,
		cooldown: 0,
		seen:     make(map[string]struct{}),
		ext:      ext,
	}
}

// ShouldSuggest checks if a suggestion should be generated for this turn.
func (s *Suggester) ShouldSuggest() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Enabled {
		return false
	}

	if s.cooldown > 0 {
		s.cooldown--
		return false
	}

	return true
}

// ResetCooldown resets the cooldown counter after a suggestion is made.
func (s *Suggester) ResetCooldown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldown = s.config.Cooldown
}

// IncrementTurn increments the turn counter (called on each turn even if not suggesting).
func (s *Suggester) IncrementTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cooldown > 0 {
		s.cooldown--
	}
}

// Generate creates a suggestion based on turn data and project context.
func (s *Suggester) Generate(ctx context.Context, turn TurnData, projCtx ProjectContext) (string, error) {
	// Build context for LLM
	var contextBuilder strings.Builder

	// Add git status context
	switch projCtx.GitStatus {
	case "dirty":
		contextBuilder.WriteString("Git status: dirty (uncommitted changes)\n")
		if len(projCtx.ModifiedFiles) > 0 && len(projCtx.ModifiedFiles) <= 5 {
			contextBuilder.WriteString("Modified files: " + strings.Join(projCtx.ModifiedFiles, ", ") + "\n")
		}
	case "clean":
		contextBuilder.WriteString("Git status: clean\n")
	default:
		contextBuilder.WriteString("Git status: not a git repo\n")
	}

	// Add last tool context
	if projCtx.LastTool != "" {
		contextBuilder.WriteString("Last tool used: " + projCtx.LastTool)
		if projCtx.LastError {
			contextBuilder.WriteString(" (resulted in error)")
		}
		contextBuilder.WriteString("\n")
	}

	// Add assistant content summary (extract text from content array)
	assistantText := extractAssistantText(turn.Assistant)
	if assistantText != "" {
		contextBuilder.WriteString("\nAssistant's last response:\n")
		contextBuilder.WriteString(truncate(assistantText, 500))
	}

	// Call LLM
	resp, err := s.ext.Chat(ctx, sdk.ChatRequest{
		System:    s.prompt,
		Messages:  []sdk.ChatMessage{{Role: "user", Content: contextBuilder.String()}},
		Model:     s.config.Model,
		MaxTokens: s.config.MaxTokens,
	})
	if err != nil {
		return "", err
	}

	suggestion := s.filter(resp.Text)
	return suggestion, nil
}

// filter applies length limits, duplicate detection, and pattern blocking.
func (s *Suggester) filter(suggestion string) string {
	suggestion = strings.TrimSpace(suggestion)

	// Truncate to 80 chars
	suggestion = truncate(suggestion, 80)

	// Block generic patterns
	lower := strings.ToLower(suggestion)
	for _, pattern := range blockedPatterns {
		if strings.Contains(lower, pattern) && len(suggestion) < 20 {
			return ""
		}
	}

	// Check for duplicates
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.ToLower(suggestion)
	if _, exists := s.seen[key]; exists {
		return ""
	}

	// Track seen suggestions (keep last 100)
	if len(s.seen) > 100 {
		// Reset seen map to prevent unbounded growth
		s.seen = make(map[string]struct{})
	}
	s.seen[key] = struct{}{}

	return suggestion
}

// extractAssistantText extracts text content from an assistant message.
func extractAssistantText(raw json.RawMessage) string {
	// Try to parse as content array
	var contentArray []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &contentArray); err == nil {
		var texts []string
		for _, c := range contentArray {
			if c.Type == "text" && c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		return strings.Join(texts, "\n")
	}

	// Try to parse as struct with content field
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err == nil {
		var texts []string
		for _, c := range msg.Content {
			if c.Type == "text" && c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		return strings.Join(texts, "\n")
	}

	// Try raw string
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}

	return ""
}

// truncate truncates s to maxRunes runes.
func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	// Find word boundary near the limit
	cutoff := maxRunes
	for i := maxRunes - 1; i > maxRunes-20 && i >= 0; i-- {
		if runes[i] == ' ' || runes[i] == '\n' {
			cutoff = i
			break
		}
	}
	return string(runes[:cutoff])
}

// ToolNames returns the list of tool names from tool results.
func (t TurnData) ToolNames() []string {
	names := make([]string, 0, len(t.ToolResults))
	for _, tr := range t.ToolResults {
		names = append(names, tr.ToolName)
	}
	return names
}

// HasError returns true if any tool result was an error.
func (t TurnData) HasError() bool {
	return slices.ContainsFunc(t.ToolResults, func(tr ToolResult) bool {
		return tr.IsError
	})
}
