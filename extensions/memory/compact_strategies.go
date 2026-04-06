package memory

import (
	"encoding/json"
	"fmt"
)

// microcompactToolResults replaces tool results outside the keepRecent window
// with a single-line summary. More aggressive than truncateToolResults — the
// full content is discarded, keeping only the tool name and original size.
func microcompactToolResults(msgs []wireMsg, keepRecent int) {
	if len(msgs) <= keepRecent {
		return
	}
	boundary := len(msgs) - keepRecent
	for i := range boundary {
		if msgs[i].Type != "tool_result" {
			continue
		}

		var tr wireToolResult
		if json.Unmarshal(msgs[i].Data, &tr) != nil {
			continue
		}

		// Calculate original size
		var origLen int
		for _, c := range tr.Content {
			origLen += len(c.Text)
		}

		// Replace content with a one-liner
		summary := fmt.Sprintf("[%s: %d chars]", tr.ToolName, origLen)
		if tr.IsError {
			summary = fmt.Sprintf("[%s: error, %d chars]", tr.ToolName, origLen)
		}
		tr.Content = tr.Content[:0]
		tr.Content = append(tr.Content, struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: summary})

		if data, err := json.Marshal(tr); err == nil {
			msgs[i].Data = data
		}
	}
}

// lightTrimMessages trims long text blocks in user and assistant messages
// outside the keepRecent window, preserving head and tail for context.
func lightTrimMessages(msgs []wireMsg, keepRecent, maxLen int) {
	if len(msgs) <= keepRecent || maxLen <= 0 {
		return
	}
	halfLen := maxLen / 2
	boundary := len(msgs) - keepRecent
	for i := range boundary {
		switch msgs[i].Type {
		case "user":
			trimUserMessage(&msgs[i], halfLen, maxLen)
		case "assistant":
			trimAssistantMessage(&msgs[i], halfLen, maxLen)
		}
	}
}

func trimUserMessage(msg *wireMsg, halfLen, maxLen int) {
	var m struct {
		Content string `json:"content"`
		Role    string `json:"role"`
	}
	if json.Unmarshal(msg.Data, &m) != nil || len([]rune(m.Content)) <= maxLen {
		return
	}
	runes := []rune(m.Content)
	m.Content = string(runes[:halfLen]) + "\n[...trimmed for compaction...]\n" + string(runes[len(runes)-halfLen:])
	if data, err := json.Marshal(m); err == nil {
		msg.Data = data
	}
}

func trimAssistantMessage(msg *wireMsg, halfLen, maxLen int) {
	// Assistant messages have content as an array of content blocks
	var m struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
		Rest json.RawMessage `json:"-"`
	}
	// Use a map to preserve all fields
	var full map[string]json.RawMessage
	if json.Unmarshal(msg.Data, &full) != nil {
		return
	}
	contentRaw, ok := full["content"]
	if !ok {
		return
	}
	if json.Unmarshal(contentRaw, &m.Content) != nil {
		return
	}

	modified := false
	for j := range m.Content {
		if m.Content[j].Type != "text" {
			continue
		}
		runes := []rune(m.Content[j].Text)
		if len(runes) > maxLen {
			m.Content[j].Text = string(runes[:halfLen]) + "\n[...trimmed for compaction...]\n" + string(runes[len(runes)-halfLen:])
			modified = true
		}
	}
	if !modified {
		return
	}
	if data, err := json.Marshal(m.Content); err == nil {
		full["content"] = data
	}
	if data, err := json.Marshal(full); err == nil {
		msg.Data = data
	}
}

// estimateTokens provides a rough token count for wire messages.
// Uses the 4-chars-per-token heuristic — accurate enough for threshold checks.
func estimateTokens(msgs []wireMsg) int {
	var total int
	for _, m := range msgs {
		total += len(m.Data)
	}
	return total / 4
}
