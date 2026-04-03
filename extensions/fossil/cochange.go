package fossil

import (
	"fmt"
	"sort"
	"strings"
)

// CoChangeEntry holds a file and the number of commits in which it co-changed
// with the target file.
type CoChangeEntry struct {
	File  string `json:"file"`
	Count int    `json:"count"`
}

// CoChange returns files that frequently change together with the given file,
// sorted by co-occurrence count descending.
func CoChange(cwd, file string, limit int) ([]CoChangeEntry, error) {
	if file == "" {
		return nil, fmt.Errorf("file is required")
	}

	// Phase 1: get commit SHAs that touched the file.
	shaOut, err := gitRun(cwd, defaultTimeout,
		"log", "--format=%h", "-100", "--", file,
	)
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	if shaOut == "" {
		return nil, nil
	}

	shas := strings.Split(shaOut, "\n")

	// Phase 2: for each commit, get all files changed (not just the target).
	counts := make(map[string]int)
	for _, sha := range shas {
		sha = strings.TrimSpace(sha)
		if sha == "" {
			continue
		}
		filesOut, err := gitRun(cwd, defaultTimeout,
			"diff-tree", "--no-commit-id", "--name-only", "-r", sha,
		)
		if err != nil {
			continue
		}
		for _, f := range strings.Split(filesOut, "\n") {
			f = strings.TrimSpace(f)
			if f != "" && f != file {
				counts[f]++
			}
		}
	}

	entries := make([]CoChangeEntry, 0, len(counts))
	for f, c := range counts {
		entries = append(entries, CoChangeEntry{File: f, Count: c})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].File < entries[j].File
	})

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return entries, nil
}
