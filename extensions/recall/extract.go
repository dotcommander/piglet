package recall

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ExtractSessionText reads a session JSONL file and returns concatenated
// user + assistant text content, truncated to maxBytes.
// Tool result entries are skipped as they are noisy and not useful for search.
func ExtractSessionText(path string, maxBytes int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open session %s: %w", path, err)
	}
	defer f.Close()

	var b strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var entry struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		if entry.Type == "meta" || len(entry.Data) == 0 {
			continue
		}

		text := extractEntryText(entry.Data)
		if text == "" {
			continue
		}

		if maxBytes > 0 && b.Len()+len(text) > maxBytes {
			remaining := maxBytes - b.Len()
			if remaining > 0 {
				runes := []rune(text)
				charCount := remaining
				if charCount > len(runes) {
					charCount = len(runes)
				}
				b.WriteString(string(runes[:charCount]))
			}
			break
		}
		b.WriteString(text)
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read session %s: %w", path, err)
	}
	return b.String(), nil
}

// extractEntryText parses a message data blob and returns "Role: content\n".
// Returns empty string for tool_result entries and entries with no text.
func extractEntryText(data json.RawMessage) string {
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(data, &msg) != nil || msg.Role == "" {
		return ""
	}

	text := extractTextContent(msg.Content)
	if text == "" {
		return ""
	}

	role := strings.Title(msg.Role) //nolint:staticcheck // acceptable for display
	return role + ": " + text + "\n"
}

// extractTextContent pulls readable text from a content field (string or []block),
// skipping tool_result blocks.
func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// String content
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}

	// Block array
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}

	var parts []string
	for _, blk := range blocks {
		if blk.Type == "text" && blk.Text != "" {
			parts = append(parts, blk.Text)
		}
		// tool_use and tool_result blocks are intentionally skipped
	}
	return strings.Join(parts, " ")
}
