// Package session — compaction reduces conversation history to save tokens.
package session

import (
	"fmt"
	"github.com/dotcommander/piglet/core"
	"strings"
	"time"
)

// CompactOptions controls compaction behavior.
type CompactOptions struct {
	KeepRecent int // number of recent messages to preserve (default 6)
}

// Compact reduces a message list by summarizing older messages.
// It keeps the first user message, summarizes the middle, and preserves recent messages.
func Compact(messages []core.Message, opts CompactOptions) []core.Message {
	if opts.KeepRecent <= 0 {
		opts.KeepRecent = 6
	}

	if len(messages) <= opts.KeepRecent+1 {
		return messages // nothing to compact
	}

	// Split: first message + middle + recent
	first := messages[0]
	cutoff := len(messages) - opts.KeepRecent
	middle := messages[1:cutoff]
	recent := messages[cutoff:]

	// Summarize middle section
	summary := summarize(middle)

	result := make([]core.Message, 0, 2+len(recent))
	result = append(result, first)
	result = append(result, &core.UserMessage{
		Content:   fmt.Sprintf("[Compacted %d messages]\n%s", len(middle), summary),
		Timestamp: time.Now(),
	})
	result = append(result, recent...)

	return result
}

func summarize(messages []core.Message) string {
	var topics []string
	seen := make(map[string]bool)

	for _, msg := range messages {
		var text string
		switch m := msg.(type) {
		case *core.UserMessage:
			text = m.Content
		case *core.AssistantMessage:
			for _, c := range m.Content {
				if tc, ok := c.(core.TextContent); ok {
					text = tc.Text
					break
				}
			}
		case *core.ToolResultMessage:
			text = m.ToolName
		}

		if text == "" {
			continue
		}

		// Extract first line as topic
		line := strings.SplitN(text, "\n", 2)[0]
		if len(line) > 80 {
			line = line[:80] + "..."
		}
		if line != "" && !seen[line] {
			seen[line] = true
			topics = append(topics, "- "+line)
		}
	}

	if len(topics) > 10 {
		topics = topics[:10]
	}

	return strings.Join(topics, "\n")
}
