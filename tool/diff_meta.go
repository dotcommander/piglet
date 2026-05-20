package tool

import "strings"

// DiffMeta is a compact summary of a file edit, attached to ToolResult.Details
// so the TUI can render "+N -N · Nf Nh" in the call tree meta column.
// It is opaque to core/ — Details is typed `any`.
type DiffMeta struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Files   int `json:"files"`
	Hunks   int `json:"hunks"`
}

// computeDiffMeta produces a DiffMeta by line-diffing before against after.
// Files is always 1 (edit/write operate on a single path). Hunks counts the
// contiguous regions that changed. A pure addition or pure deletion still
// counts as one hunk.
func computeDiffMeta(before, after string) DiffMeta {
	beforeLines := splitLines(before)
	afterLines := splitLines(after)

	added, removed, hunks := diffLines(beforeLines, afterLines)
	return DiffMeta{
		Added:   added,
		Removed: removed,
		Files:   1,
		Hunks:   hunks,
	}
}

// splitLines splits text into lines, treating an empty string as zero lines
// (not one empty line) so a freshly created file diffs cleanly from "".
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diffLines runs an LCS line diff and returns added lines, removed lines, and
// the number of contiguous change regions (hunks). Equal runs separate hunks.
func diffLines(a, b []string) (added, removed, hunks int) {
	lcs := lcsTable(a, b)
	i, j := 0, 0
	inHunk := false
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i++
			j++
			inHunk = false
		case j < len(b) && (i >= len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			added++
			j++
			if !inHunk {
				hunks++
				inHunk = true
			}
		default:
			removed++
			i++
			if !inHunk {
				hunks++
				inHunk = true
			}
		}
	}
	return added, removed, hunks
}

// lcsTable builds the longest-common-subsequence length table for line slices a
// and b. lcs[i][j] is the LCS length of a[i:] and b[j:].
func lcsTable(a, b []string) [][]int {
	rows, cols := len(a)+1, len(b)+1
	lcs := make([][]int, rows)
	for i := range lcs {
		lcs[i] = make([]int, cols)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	return lcs
}
