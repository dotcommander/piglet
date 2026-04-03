package memory

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	contextCategory = "_context"
	maxContextFacts = 50
	pruneCount      = 10
)

// TurnData represents the relevant data from an EventTurnEnd, parsed from JSON.
type TurnData struct {
	Assistant   json.RawMessage   `json:"Assistant"`
	ToolResults []json.RawMessage `json:"ToolResults"`
}

// toolCall represents a tool call from an assistant message.
type toolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// toolResult represents a tool result message.
type toolResult struct {
	ToolName   string `json:"tool_name"`
	ToolCallID string `json:"tool_call_id"`
	IsError    bool   `json:"is_error"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// assistantMsg represents an assistant message (partial parse).
type assistantMsg struct {
	Content []json.RawMessage `json:"content"`
}

// Extractor writes structured facts from each turn to the memory store.
type Extractor struct {
	store   *Store
	turnNum int
}

// NewExtractor creates an extractor backed by the given store.
func NewExtractor(store *Store) *Extractor {
	return &Extractor{store: store}
}

// Extract parses turn data and writes facts to the store.
// It's designed to be called from an EventTurnEnd handler.
func (e *Extractor) Extract(eventData json.RawMessage) error {
	var turn TurnData
	if err := json.Unmarshal(eventData, &turn); err != nil {
		return nil // silently skip unparseable events
	}
	e.turnNum++

	// Extract from tool results
	for _, raw := range turn.ToolResults {
		var tr toolResult
		if json.Unmarshal(raw, &tr) != nil {
			continue
		}
		e.extractToolResult(tr)
	}

	// Enforce cap
	e.pruneIfNeeded()
	return nil
}

func (e *Extractor) extractToolResult(tr toolResult) {
	text := e.resultText(tr)

	switch tr.ToolName {
	case "Read":
		// Extract file path from the result text (first line often has the path)
		if path := extractFilePath(text); path != "" {
			_ = e.store.Set("ctx:file:"+path, truncRunes(text, 300), contextCategory)
		}
	case "Grep", "Glob":
		if path := extractFilePath(text); path != "" {
			_ = e.store.Set("ctx:search:"+path, truncRunes(text, 300), contextCategory)
		}
	case "Edit", "Write":
		if path := extractFilePath(text); path != "" {
			_ = e.store.Set("ctx:edit:"+path, truncRunes(text, 200), contextCategory)
		}
	case "Bash":
		key := fmt.Sprintf("ctx:cmd:%d", e.turnNum)
		if tr.IsError {
			key = fmt.Sprintf("ctx:error:%d", e.turnNum)
		}
		_ = e.store.Set(key, truncRunes(text, 300), contextCategory)
	default:
		// For other tools, store a brief record
		if text != "" {
			key := fmt.Sprintf("ctx:tool:%s:%d", tr.ToolName, e.turnNum)
			_ = e.store.Set(key, truncRunes(text, 200), contextCategory)
		}
	}
}

func (e *Extractor) resultText(tr toolResult) string {
	for _, c := range tr.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text
		}
	}
	return ""
}

func (e *Extractor) pruneIfNeeded() {
	facts := e.store.List(contextCategory)
	if len(facts) <= maxContextFacts {
		return
	}

	// Sort by UpdatedAt ascending, prune oldest
	slices.SortFunc(facts, func(a, b Fact) int {
		return a.UpdatedAt.Compare(b.UpdatedAt)
	})

	for i := range min(pruneCount, len(facts)) {
		_ = e.store.Delete(facts[i].Key)
	}
}

// extractFilePath tries to find a file path in tool result text.
// Looks for common patterns: absolute paths, or the first line if it looks like a path.
func extractFilePath(text string) string {
	// First line
	line, _, _ := strings.Cut(text, "\n")
	line = strings.TrimSpace(line)

	// Common patterns from Read/Edit/Write results
	// "The file /path/to/file has been updated"
	// "1→ content..." (Read output starts with line numbers)
	if strings.HasPrefix(line, "/") {
		// Absolute path — take until space
		if idx := strings.IndexByte(line, ' '); idx > 0 {
			return line[:idx]
		}
		return line
	}

	// Check for "file_path" in structured messages
	if strings.Contains(text, "/") {
		// Look for path-like tokens
		for _, word := range strings.Fields(line) {
			if strings.Count(word, "/") >= 2 && !strings.HasPrefix(word, "http") {
				return strings.Trim(word, "\"'`,;:")
			}
		}
	}

	return ""
}

// truncRunes truncates s to n runes, appending "..." if truncated.
func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
