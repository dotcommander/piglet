package fossil

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BlameEntry represents a unique commit that touches a blamed line range.
type BlameEntry struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Summary string `json:"summary"`
	Lines   string `json:"lines"` // e.g. "42-58" or "42"
}

// Why returns the blame history for the given file and optional line range.
// If startLine and endLine are both 0, the entire file is blamed.
func Why(cwd, file string, startLine, endLine int) ([]BlameEntry, error) {
	args := []string{"blame", "-p"}
	if startLine != 0 || endLine != 0 {
		args = append(args, "-L", fmt.Sprintf("%d,%d", startLine, endLine))
	}
	args = append(args, "--", file)

	out, err := gitRun(cwd, defaultTimeout, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	return parsePorcelainBlame(out)
}

// commitInfo holds the fields we care about while parsing a porcelain block.
type commitInfo struct {
	author     string
	authorTime int64
	summary    string
}

// parsePorcelainBlame parses the output of `git blame -p` and returns
// deduplicated BlameEntry values sorted by first line number ascending.
func parsePorcelainBlame(output string) ([]BlameEntry, error) {
	// Per-SHA metadata collected during parsing.
	meta := map[string]*commitInfo{}
	// Map from SHA to the set of final line numbers it touches.
	shaLines := map[string][]int{}
	// Track insertion order so we can preserve stable sort fallback.
	var shaOrder []string

	lines := strings.Split(output, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]

		// A blame block header: 40 hex chars, then orig-line, final-line, [count].
		if len(line) >= 40 && isHexString(line[:40]) {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				i++
				continue
			}
			sha := fields[0]
			finalLine, err := strconv.Atoi(fields[2])
			if err != nil {
				i++
				continue
			}

			if _, seen := meta[sha]; !seen {
				meta[sha] = &commitInfo{}
				shaOrder = append(shaOrder, sha)
			}
			shaLines[sha] = append(shaLines[sha], finalLine)
			i++

			// Consume header key-value lines until the tab-prefixed content line.
			for i < len(lines) {
				hdr := lines[i]
				if strings.HasPrefix(hdr, "\t") {
					// Content line — block is complete.
					i++
					break
				}
				switch {
				case strings.HasPrefix(hdr, "author "):
					meta[sha].author = strings.TrimPrefix(hdr, "author ")
				case strings.HasPrefix(hdr, "author-time "):
					ts, _ := strconv.ParseInt(strings.TrimPrefix(hdr, "author-time "), 10, 64)
					meta[sha].authorTime = ts
				case strings.HasPrefix(hdr, "summary "):
					meta[sha].summary = strings.TrimPrefix(hdr, "summary ")
				}
				i++
			}
			continue
		}
		i++
	}

	entries := make([]BlameEntry, 0, len(shaOrder))
	for _, sha := range shaOrder {
		info := meta[sha]
		lineNums := shaLines[sha]

		date := ""
		if info.authorTime != 0 {
			date = time.Unix(info.authorTime, 0).UTC().Format("2006-01-02")
		}

		entries = append(entries, BlameEntry{
			SHA:     sha[:8],
			Author:  info.author,
			Date:    date,
			Summary: info.summary,
			Lines:   lineRange(lineNums),
		})
	}

	// Sort by first line number ascending.
	sort.Slice(entries, func(a, b int) bool {
		aMin := minLine(shaLines[expandSHA(entries[a].SHA, shaOrder)])
		bMin := minLine(shaLines[expandSHA(entries[b].SHA, shaOrder)])
		return aMin < bMin
	})

	return entries, nil
}

// lineRange formats a slice of line numbers as "min-max" or "min" if all equal.
func lineRange(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	mn, mx := nums[0], nums[0]
	for _, n := range nums[1:] {
		if n < mn {
			mn = n
		}
		if n > mx {
			mx = n
		}
	}
	if mn == mx {
		return strconv.Itoa(mn)
	}
	return fmt.Sprintf("%d-%d", mn, mx)
}

func minLine(nums []int) int {
	if len(nums) == 0 {
		return 0
	}
	m := nums[0]
	for _, n := range nums[1:] {
		if n < m {
			m = n
		}
	}
	return m
}

// expandSHA maps a short (8-char) SHA back to the full SHA for shaLines lookup.
func expandSHA(short string, order []string) string {
	for _, full := range order {
		if strings.HasPrefix(full, short) {
			return full
		}
	}
	return short
}

// isHexString reports whether s contains only hexadecimal characters.
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
