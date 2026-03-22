package repomap

import (
	"path/filepath"
	"slices"
	"strings"
)

// RankedFile is a FileSymbols with an importance score.
type RankedFile struct {
	FileSymbols
	Score int    // higher = more important
	Tag   string // e.g. "entry", ""
}

// RankFiles scores and sorts files by importance.
// Returns files sorted by score descending, then by path ascending for ties.
func RankFiles(files []*FileSymbols) []RankedFile {
	ranked := make([]RankedFile, len(files))
	for i, f := range files {
		ranked[i] = RankedFile{FileSymbols: *f}
	}

	applyEntryBoosts(ranked)
	applySymbolBonus(ranked)
	applyDepthPenalty(ranked)
	applyReferenceCounts(ranked, files)

	slices.SortFunc(ranked, func(a, b RankedFile) int {
		if b.Score != a.Score {
			return b.Score - a.Score
		}
		return strings.Compare(a.Path, b.Path)
	})

	return ranked
}

func applyEntryBoosts(ranked []RankedFile) {
	for i := range ranked {
		path := ranked[i].Path
		base := filepath.Base(path)

		// Go entry points
		if base == "main.go" {
			ranked[i].Score += 50
			ranked[i].Tag = "entry"
			continue
		}

		// Other language entry points
		switch base {
		case "main.ts", "index.ts", "index.js", "app.py", "main.py", "main.rs":
			ranked[i].Score += 30
			ranked[i].Tag = "entry"
		}
	}
}

func applySymbolBonus(ranked []RankedFile) {
	for i := range ranked {
		for _, sym := range ranked[i].Symbols {
			if sym.Exported {
				ranked[i].Score++
			}
		}
	}
}

func applyDepthPenalty(ranked []RankedFile) {
	for i := range ranked {
		depth := strings.Count(ranked[i].Path, string(filepath.Separator))
		if depth > 2 {
			ranked[i].Score -= depth - 2
		}
	}
}

func applyReferenceCounts(ranked []RankedFile, files []*FileSymbols) {
	if len(files) == 0 {
		return
	}

	if files[0].Language == "go" {
		applyGoReferenceCounts(ranked, files)
	} else {
		applyBasenameReferenceCounts(ranked, files)
	}
}

// applyGoReferenceCounts scores Go files by how many other files import their package.
func applyGoReferenceCounts(ranked []RankedFile, files []*FileSymbols) {
	// Map importPath → index in ranked slice.
	importIndex := make(map[string]int, len(files))
	for i, f := range files {
		if f.ImportPath != "" {
			importIndex[f.ImportPath] = i
		}
	}

	for _, f := range files {
		for _, imp := range f.Imports {
			if idx, ok := importIndex[imp]; ok {
				ranked[idx].Score += 10
			}
		}
	}
}

// applyBasenameReferenceCounts scores non-Go files by basename matching in imports.
func applyBasenameReferenceCounts(ranked []RankedFile, files []*FileSymbols) {
	// Map basename (without ext) → index in ranked slice.
	basenameIndex := make(map[string]int, len(files))
	for i, f := range files {
		name := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
		basenameIndex[name] = i
	}

	for _, f := range files {
		for _, imp := range f.Imports {
			// Normalize the import to its last path segment, without extension.
			seg := filepath.Base(imp)
			seg = strings.TrimSuffix(seg, filepath.Ext(seg))
			if idx, ok := basenameIndex[seg]; ok {
				ranked[idx].Score += 10
			}
		}
	}
}
