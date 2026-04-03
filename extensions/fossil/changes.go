package fossil

import (
	"strconv"
	"strings"
	"unicode"
)

// ChangeSummary represents a single commit with its file-level change stats.
type ChangeSummary struct {
	SHA     string       `json:"sha"`
	Author  string       `json:"author"`
	Subject string       `json:"subject"`
	Files   []FileChange `json:"files"`
}

// FileChange holds the per-file add/delete counts for a commit.
type FileChange struct {
	File    string `json:"file"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

// Changes returns commits reachable since the given duration string (e.g. "7d",
// "24h") optionally filtered to a path. If since is empty it defaults to "7d".
func Changes(cwd, since, path string) ([]ChangeSummary, error) {
	if since == "" {
		since = "7 days ago"
	} else {
		since = normalizeSince(since)
	}

	args := []string{
		"log",
		"--numstat",
		`--format=format:%h|%an|%s`,
		"--since=" + since,
	}
	if path != "" {
		args = append(args, "--", path)
	}

	out, err := gitRun(cwd, defaultTimeout, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	return parseNumstatLog(out), nil
}

// parseNumstatLog parses the mixed format produced by
// `git log --numstat --format=format:%h|%an|%s`.
//
// The output interleaves commit header lines with numstat file lines:
//
//	abc1234|Alice|Fix the thing
//	3	1	pkg/foo.go
//	-	-	pkg/binary.bin
//
//	def5678|Bob|Add feature
//	10	0	pkg/bar.go
func parseNumstatLog(output string) []ChangeSummary {
	var results []ChangeSummary
	var current *ChangeSummary

	for _, line := range strings.Split(output, "\n") {
		// Skip blank lines between commits.
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Commit header: exactly 3 pipe-separated fields where the first looks
		// like a short SHA (alphanumeric, 7-12 chars). We check for the pipe
		// delimiter to distinguish from numstat lines which use tabs.
		if !strings.Contains(line, "\t") && strings.Contains(line, "|") {
			parts := strings.SplitN(line, "|", 3)
			if len(parts) == 3 {
				if current != nil {
					results = append(results, *current)
				}
				current = &ChangeSummary{
					SHA:     parts[0],
					Author:  parts[1],
					Subject: parts[2],
				}
				continue
			}
		}

		// Numstat line: <added>\t<deleted>\t<file>
		if current != nil && strings.Contains(line, "\t") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) == 3 {
				fc := FileChange{File: parts[2]}
				if parts[0] == "-" {
					fc.Added = 0
				} else {
					fc.Added, _ = strconv.Atoi(parts[0])
				}
				if parts[1] == "-" {
					fc.Deleted = 0
				} else {
					fc.Deleted, _ = strconv.Atoi(parts[1])
				}
				current.Files = append(current.Files, fc)
			}
		}
	}

	if current != nil {
		results = append(results, *current)
	}

	return results
}

// normalizeSince converts shorthand like "7d", "30d", "2w" into git-friendly
// duration strings like "7 days ago", "30 days ago", "2 weeks ago".
// If the input doesn't match a known pattern, it's returned as-is (allowing
// git-native formats like "2025-01-01" or "3 months ago").
func normalizeSince(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "7 days ago"
	}

	// Split into numeric prefix and unit suffix.
	i := 0
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		i++
	}
	if i == 0 || i == len(s) {
		return s // no numeric prefix or no unit — pass through
	}

	num := s[:i]
	unit := strings.ToLower(s[i:])

	switch unit {
	case "d", "day", "days":
		return num + " days ago"
	case "w", "week", "weeks":
		return num + " weeks ago"
	case "m", "month", "months":
		return num + " months ago"
	case "y", "year", "years":
		return num + " years ago"
	case "h", "hour", "hours":
		return num + " hours ago"
	default:
		return s
	}
}
