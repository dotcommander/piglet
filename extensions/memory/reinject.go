package memory

import (
	"fmt"
	"slices"
	"strings"
)

const (
	reinjectMaxTokens   = 50000
	reinjectMaxPerItem  = 5000
	reinjectMaxItems    = 5
	reinjectCharsPerTok = 4
)

// criticalContext holds a re-injectable context item after compaction.
type criticalContext struct {
	category string
	content  string
}

// gatherCriticalContext reads the memory store for context that should survive compaction.
// It collects plan content, recent edits, and other high-value facts.
func gatherCriticalContext(s *Store) []criticalContext {
	var items []criticalContext
	facts := s.List(contextCategory)

	// Collect recent edits (last 3 by UpdatedAt)
	editFacts := filterFacts(facts, "ctx:edit:")
	items = appendItems(items, editFacts, 3, "recent edits")

	// Collect plan facts
	planFacts := filterFacts(facts, "ctx:plan:")
	items = appendItems(items, planFacts, 1, "active plan")

	// Collect error context (useful for avoiding repeated mistakes)
	errorFacts := filterFacts(facts, "ctx:error:")
	items = appendItems(items, errorFacts, 1, "recent errors")

	return items
}

// filterFacts returns facts whose key starts with the given prefix,
// sorted by UpdatedAt descending so the most recently updated facts appear first.
func filterFacts(facts []Fact, prefix string) []Fact {
	var out []Fact
	for _, f := range facts {
		if strings.HasPrefix(f.Key, prefix) {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b Fact) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	return out
}

// appendItems adds up to max items from facts to items, with budget enforcement.
func appendItems(items []criticalContext, facts []Fact, max int, category string) []criticalContext {
	if len(items) >= reinjectMaxItems {
		return items
	}

	remaining := reinjectMaxTokens - totalTokens(items)
	if remaining <= 0 {
		return items
	}

	limit := min(max, len(facts))
	for i := 0; i < limit && len(items) < reinjectMaxItems; i++ {
		f := facts[i]
		maxChars := min(reinjectMaxPerItem*reinjectCharsPerTok, remaining*reinjectCharsPerTok)
		if maxChars <= 0 {
			break
		}

		content := f.Value
		if len(content) > maxChars {
			runes := []rune(content)
			content = string(runes[:maxChars]) + "..."
		}

		items = append(items, criticalContext{
			category: category,
			content:  fmt.Sprintf("%s: %s", f.Key, content),
		})
		remaining = reinjectMaxTokens - totalTokens(items)
	}

	return items
}

// totalTokens estimates the total token count across all items.
func totalTokens(items []criticalContext) int {
	total := 0
	for _, item := range items {
		total += len(item.content) / reinjectCharsPerTok
	}
	return total
}

// buildReinjectMessage formats critical context items into a user message
// to be appended after the compaction summary.
func buildReinjectMessage(items []criticalContext) string {
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("[Post-compact context preservation]\n")
	for _, item := range items {
		fmt.Fprintf(&b, "%s: %s\n", item.category, item.content)
	}
	return b.String()
}
