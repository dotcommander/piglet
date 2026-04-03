package fossil

import (
	"fmt"
	"strings"
)

// Log returns a token-budgeted git log for the given path (or the whole repo
// if path is empty). Output is plain text suitable for agent consumption.
func Log(cwd string, tokens int, path string) (string, error) {
	if tokens <= 0 {
		tokens = 1024
	}
	byteBudget := tokens * 4

	args := []string{"log", "--oneline", "--no-merges"}
	if path != "" {
		args = append(args, "--", path)
	}

	out, err := gitRun(cwd, defaultTimeout, args...)
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}

	lines := strings.Split(out, "\n")
	// Drop trailing empty line that split often produces.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	const maxLines = 500
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	// Accumulate lines up to the byte budget.
	var b strings.Builder
	truncated := false
	for _, line := range lines {
		needed := len(line) + 1 // +1 for newline
		if b.Len()+needed > byteBudget {
			truncated = true
			break
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}

	result := b.String()
	if truncated {
		result += fmt.Sprintf("[... truncated to ~%d tokens]\n", tokens)
	}

	return result, nil
}
