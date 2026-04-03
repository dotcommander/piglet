package sessiontools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// sessionEntry represents a single line in a session JSONL file.
type sessionEntry struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// QuerySession searches a session JSONL file for lines matching the query string.
// Returns matching content excerpts up to maxSize bytes of file.
func QuerySession(path, query string, maxSize int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat session file: %w", err)
	}
	if info.Size() > maxSize {
		return "", fmt.Errorf("session file too large (%d bytes, max %d)", info.Size(), maxSize)
	}

	q := strings.ToLower(query)
	var b strings.Builder
	matchCount := 0
	const maxMatches = 50

	scanner := bufio.NewScanner(f)
	// Allow long lines in session files
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		if matchCount >= maxMatches {
			fmt.Fprintf(&b, "\n... (%d+ matches, showing first %d)", matchCount, maxMatches)
			break
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry sessionEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		content := extractContent(entry.Content)
		if content == "" {
			continue
		}

		if strings.Contains(strings.ToLower(content), q) {
			matchCount++
			// Truncate long content for display
			runes := []rune(content)
			if len(runes) > 500 {
				content = string(runes[:500]) + "..."
			}

			role := entry.Role
			if role == "" {
				role = entry.Type
			}
			fmt.Fprintf(&b, "[%s] %s\n\n", role, content)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan session file: %w", err)
	}

	if matchCount == 0 {
		return fmt.Sprintf("No matches for %q in session.", query), nil
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// extractContent gets a string from the content field, which may be a JSON
// string, an object with a text field, or an array of content blocks.
func extractContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	case '{':
		var obj struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &obj); err == nil {
			return obj.Text
		}
	case '[':
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &blocks); err == nil {
			var parts []string
			for _, block := range blocks {
				if block.Text != "" {
					parts = append(parts, block.Text)
				}
			}
			return strings.Join(parts, "\n")
		}
	}

	return ""
}
