package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/piglet/core"
)

const keepRecent = 6

// CompactFn returns a compaction function backed by the given memory store.
// If prov is non-nil, it makes a short LLM call to refine the extracted facts
// into a concise summary. If prov is nil, it uses the raw facts directly.
func CompactFn(store *Store, prov core.StreamProvider) func(context.Context, []core.Message) ([]core.Message, error) {
	return func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		if len(msgs) <= keepRecent+1 {
			return msgs, nil
		}

		// Read accumulated context facts
		facts := store.List(contextCategory)
		summary := buildFactSummary(facts)

		// If we have a provider, refine with a short LLM call
		if prov != nil && summary != "" {
			refined := refineSummary(ctx, prov, summary)
			if refined != "" {
				summary = refined
			}
		}

		// Write summary back to store (replaces granular facts with one summary)
		if summary != "" {
			_ = store.Set("ctx:summary", summary, contextCategory)
		}

		// Build the compacted message set
		var reference strings.Builder
		reference.WriteString("[Context compacted — session memory updated]\n\n")
		reference.WriteString("Use memory_list category=_context to see accumulated context.\n")
		reference.WriteString("Use memory_get to retrieve specific facts.\n")
		if summary != "" {
			reference.WriteString("\nSummary: ")
			reference.WriteString(summary)
		}

		return core.CompactMessages(msgs, reference.String()), nil
	}
}

// buildFactSummary creates a text representation of context facts grouped by type.
func buildFactSummary(facts []Fact) string {
	if len(facts) == 0 {
		return ""
	}

	var files, edits, errors, cmds, other []string
	for _, f := range facts {
		switch {
		case strings.HasPrefix(f.Key, "ctx:file:"):
			path := strings.TrimPrefix(f.Key, "ctx:file:")
			files = append(files, path)
		case strings.HasPrefix(f.Key, "ctx:edit:"):
			path := strings.TrimPrefix(f.Key, "ctx:edit:")
			edits = append(edits, path+": "+firstLine(f.Value))
		case strings.HasPrefix(f.Key, "ctx:error:"):
			errors = append(errors, firstLine(f.Value))
		case strings.HasPrefix(f.Key, "ctx:cmd:"):
			cmds = append(cmds, firstLine(f.Value))
		case strings.HasPrefix(f.Key, "ctx:summary"):
			// Skip existing summaries
		default:
			other = append(other, f.Key+": "+firstLine(f.Value))
		}
	}

	var b strings.Builder
	if len(files) > 0 {
		fmt.Fprintf(&b, "Files read: %s\n", strings.Join(files, ", "))
	}
	if len(edits) > 0 {
		b.WriteString("Edits:\n")
		for _, e := range edits {
			fmt.Fprintf(&b, "  - %s\n", e)
		}
	}
	if len(errors) > 0 {
		b.WriteString("Errors encountered:\n")
		for _, e := range errors {
			fmt.Fprintf(&b, "  - %s\n", e)
		}
	}
	if len(cmds) > 0 {
		b.WriteString("Commands:\n")
		for _, c := range cmds {
			fmt.Fprintf(&b, "  - %s\n", c)
		}
	}
	if len(other) > 0 {
		b.WriteString("Other:\n")
		for _, o := range other {
			fmt.Fprintf(&b, "  - %s\n", o)
		}
	}
	return b.String()
}

// refineSummary makes a short LLM call to produce a concise summary from structured facts.
func refineSummary(ctx context.Context, prov core.StreamProvider, factText string) string {
	ch := prov.Stream(ctx, core.StreamRequest{
		System: "Given these extracted facts from a coding session, produce a concise structured summary. Group by: files touched, decisions made, errors resolved, current task state. Be brief — 5-10 lines max.",
		Messages: []core.Message{
			&core.UserMessage{Content: factText, Timestamp: time.Now()},
		},
	})

	var out strings.Builder
	for evt := range ch {
		if evt.Type == core.StreamTextDelta {
			out.WriteString(evt.Delta)
		}
		if evt.Type == core.StreamError {
			return "" // fall back to raw facts
		}
	}
	return out.String()
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	r := []rune(line)
	if len(r) > 100 {
		return string(r[:100]) + "..."
	}
	return line
}

// contextCounts holds categorized counts of context facts.
type contextCounts struct {
	files, edits, errors, cmds int
}

// countContextFacts categorizes context facts by key prefix.
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
