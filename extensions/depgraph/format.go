package depgraph

import (
	"fmt"
	"strings"
)

func shorten(path, module string) string {
	if s, ok := strings.CutPrefix(path, module+"/"); ok {
		return s
	}
	return path
}

// FormatDeps formats a dependency list as an indented tree.
// root is shown on the first line at full path; entries are indented depth*2 spaces.
// module prefixes are stripped from all entry paths for compact display.
// If tokens > 0, output is truncated at a line boundary to fit the byte budget.
func FormatDeps(root, module string, entries []DepEntry, tokens int) string {
	var b strings.Builder
	budget := 0
	if tokens > 0 {
		budget = tokens * 4
	}

	b.WriteString(root)

	for i, e := range entries {
		line := "\n" + strings.Repeat(" ", e.Depth*2) + shorten(e.ImportPath, module)
		if budget > 0 && b.Len()+len(line) > budget {
			remaining := len(entries) - i
			fmt.Fprintf(&b, "\n... (%d more)", remaining)
			return b.String()
		}
		b.WriteString(line)
	}

	return b.String()
}

// FormatImpact formats an impact list with a header count.
// module prefixes are stripped; output is truncated to the token budget if tokens > 0.
func FormatImpact(packages []string, module string, tokens int) string {
	var b strings.Builder
	budget := 0
	if tokens > 0 {
		budget = tokens * 4
	}

	fmt.Fprintf(&b, "Impact: %d package", len(packages))
	if len(packages) != 1 {
		b.WriteString("s")
	}

	for i, p := range packages {
		line := "\n  " + shorten(p, module)
		if budget > 0 && b.Len()+len(line) > budget {
			remaining := len(packages) - i
			fmt.Fprintf(&b, "\n... (%d more)", remaining)
			return b.String()
		}
		b.WriteString(line)
	}

	return b.String()
}

// FormatCycles formats cycle detection results.
func FormatCycles(cycles []Cycle, module string) string {
	if len(cycles) == 0 {
		return "No cycles detected"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d cycle", len(cycles))
	if len(cycles) != 1 {
		b.WriteString("s")
	}
	b.WriteString(" detected")

	for _, c := range cycles {
		b.WriteString("\n\n  ")
		parts := make([]string, len(c.Path))
		for i, p := range c.Path {
			parts[i] = shorten(p, module)
		}
		// Close the cycle by repeating the first node.
		b.WriteString(strings.Join(parts, " \u2192 "))
		b.WriteString(" \u2192 ")
		b.WriteString(parts[0])
	}

	return b.String()
}

// FormatPath formats a shortest-path result as an arrow-joined chain.
func FormatPath(path []string, module string) string {
	if len(path) == 0 {
		return "No path found"
	}

	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = shorten(p, module)
	}
	return strings.Join(parts, " \u2192 ")
}
