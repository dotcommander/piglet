package memory

import "strings"

// contextCounts holds categorized counts of context facts.
type contextCounts struct {
	files, edits, errors, cmds int
}

// countContextFacts categorizes context facts by key prefix.
// Used by BuildMemoryPrompt to generate a brief context summary.
func countContextFacts(facts []Fact) contextCounts {
	var c contextCounts
	for _, f := range facts {
		switch {
		case strings.HasPrefix(f.Key, "ctx:file:") || strings.HasPrefix(f.Key, "ctx:search:"):
			c.files++
		case strings.HasPrefix(f.Key, "ctx:edit:"):
			c.edits++
		case strings.HasPrefix(f.Key, "ctx:error:"):
			c.errors++
		case strings.HasPrefix(f.Key, "ctx:cmd:"):
			c.cmds++
		}
	}
	return c
}
