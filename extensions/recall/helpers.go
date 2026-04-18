package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// resolveSessionMeta returns the path and title for sessionID by looking it up
// in e.Sessions. Falls back to empty strings if sessions cannot be fetched.
func resolveSessionMeta(ctx context.Context, e *sdk.Extension, sessionID string) (path, title string) {
	sessions, err := e.Sessions(ctx)
	if err != nil {
		return "", ""
	}
	for _, s := range sessions {
		if s.ID == sessionID {
			return s.Path, s.Title
		}
	}
	return "", ""
}

// formatMessagesText converts EventAgentEnd messages to a plain text string.
func formatMessagesText(messages []json.RawMessage) string {
	var b strings.Builder
	for _, raw := range messages {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &msg) != nil || msg.Role == "" {
			continue
		}
		text := extractTextContent(msg.Content)
		if text == "" {
			continue
		}
		role := cases.Title(language.English).String(msg.Role)
		fmt.Fprintf(&b, "%s: %s\n", role, text)
	}
	return b.String()
}

// buildResultsText formats search results as a readable string.
func buildResultsText(results []SearchResult) string {
	var b strings.Builder
	for i, r := range results {
		label := r.Title
		if label == "" {
			label = r.SessionID
			if len(label) > 8 {
				label = label[:8]
			}
		}
		fmt.Fprintf(&b, "%d. %s (score: %.4f)\n", i+1, label, r.Score)
		excerpt := readExcerpt(r.Path, searchExcerptLen)
		if excerpt != "" {
			fmt.Fprintf(&b, "   %s\n", strings.ReplaceAll(strings.TrimSpace(excerpt), "\n", " "))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// readExcerpt reads the first maxChars characters of text from the session file.
func readExcerpt(path string, maxChars int) string {
	if path == "" {
		return ""
	}
	text, err := ExtractSessionText(path, maxChars*4) // bytes approx
	if err != nil {
		return ""
	}
	runes := []rune(text)
	if len(runes) > maxChars {
		return string(runes[:maxChars])
	}
	return text
}

// wordCount returns the approximate number of words in s.
func wordCount(s string) int {
	return len(strings.Fields(s))
}
