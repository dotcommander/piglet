package fossil

import (
	"fmt"
	"strconv"
	"strings"
)

// OwnerEntry holds commit authorship statistics for a single author.
type OwnerEntry struct {
	Author  string  `json:"author"`
	Commits int     `json:"commits"`
	Percent float64 `json:"percent"`
}

// Owners returns the top commit authors for the given path (or the whole repo
// if path is empty), sorted by commit count descending.
func Owners(cwd, path string, limit int) ([]OwnerEntry, error) {
	args := []string{"shortlog", "-sn", "--no-merges", "HEAD"}
	if path != "" {
		args = append(args, "--", path)
	}

	out, err := gitRun(cwd, defaultTimeout, args...)
	if err != nil {
		return nil, fmt.Errorf("git shortlog: %w", err)
	}

	var entries []OwnerEntry
	total := 0

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		countStr, name, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		countStr = strings.TrimSpace(countStr)
		name = strings.TrimSpace(name)
		count, err := strconv.Atoi(countStr)
		if err != nil {
			continue
		}
		total += count
		entries = append(entries, OwnerEntry{Author: name, Commits: count})
	}

	if total > 0 {
		for i := range entries {
			entries[i].Percent = float64(entries[i].Commits) / float64(total) * 100
		}
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return entries, nil
}
