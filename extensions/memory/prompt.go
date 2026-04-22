package memory

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"
)

//go:embed defaults/tool-preamble.md
var defaultToolPreamble string

const promptContentCap = 8000

// BuildMemoryPrompt generates the prompt section content from memory store.
func BuildMemoryPrompt(store *Store) string {
	var b strings.Builder

	b.WriteString(defaultPreamble() + "\n\n")

	allFacts := store.List("")
	if len(allFacts) == 0 {
		b.WriteString("No memories stored yet.")
		return b.String()
	}

	// Partition user facts vs context facts
	var userFacts, contextFacts []Fact
	for _, f := range allFacts {
		if f.Category == "_context" {
			contextFacts = append(contextFacts, f)
		} else {
			userFacts = append(userFacts, f)
		}
	}

	// User facts first (full display)
	if len(userFacts) > 0 {
		b.WriteString("Current memories:\n")

		// Sort by importance desc, key asc — highest-importance facts survive cap trimming.
		slices.SortFunc(userFacts, func(a, b Fact) int {
			if a.Importance != b.Importance {
				return b.Importance - a.Importance
			}
			return strings.Compare(a.Key, b.Key)
		})

		lines := make([]string, len(userFacts))
		for i, f := range userFacts {
			if f.Category != "" {
				lines[i] = "- " + f.Key + ": " + f.Value + " (" + f.Category + ")"
			} else {
				lines[i] = "- " + f.Key + ": " + f.Value
			}
		}

		// Walk forward until next line would exceed cap; drop remaining (lowest-importance) entries.
		userCap := promptContentCap - 500 // reserve space for context section
		end := len(lines)
		total := 0
		for i, l := range lines {
			if total+len(l)+1 > userCap {
				end = i
				break
			}
			total += len(l) + 1
		}

		for _, l := range lines[:end] {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}

	// Context facts as brief summary
	if len(contextFacts) > 0 {
		// Check for a stored summary first
		if summary, ok := store.Get("ctx:summary"); ok {
			b.WriteString("\nSession context:\n")
			b.WriteString(summary.Value)
			b.WriteByte('\n')
		} else {
			counts := countContextFacts(contextFacts)
			files, edits, errors, cmds := counts.files, counts.edits, counts.errors, counts.cmds
			b.WriteString("\nSession context (use memory_get for details):\n")
			if files > 0 {
				fmt.Fprintf(&b, "- %d file(s) read/searched\n", files)
			}
			if edits > 0 {
				fmt.Fprintf(&b, "- %d file(s) edited\n", edits)
			}
			if errors > 0 {
				fmt.Fprintf(&b, "- %d error(s) encountered\n", errors)
			}
			if cmds > 0 {
				fmt.Fprintf(&b, "- %d command(s) run\n", cmds)
			}
		}
	}

	return b.String()
}

func defaultPreamble() string {
	return strings.TrimSpace(defaultToolPreamble)
}
