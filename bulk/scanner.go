package bulk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Scanner discovers items from a source.
type Scanner interface {
	Scan(ctx context.Context) ([]Item, error)
}

// DirScanner finds subdirectories under Root, optionally filtering with Match.
type DirScanner struct {
	Root  string
	Depth int                    // default 1
	Match func(path string) bool // nil = all directories
}

// Scan implements Scanner for DirScanner.
func (s *DirScanner) Scan(_ context.Context) ([]Item, error) {
	depth := s.Depth
	if depth < 1 {
		depth = 1
	}

	root, err := filepath.Abs(s.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve dir %q: %w", s.Root, err)
	}

	var paths []string
	if err := walkDirs(root, 0, depth, s.Match, &paths); err != nil {
		return nil, err
	}

	slices.Sort(paths)
	items := make([]Item, len(paths))
	for i, p := range paths {
		items[i] = Item{Name: filepath.Base(p), Path: p}
	}
	return items, nil
}

// walkDirs recursively visits directories looking for matches.
func walkDirs(dir string, current, max int, match func(string) bool, results *[]string) error {
	if current >= max {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // skip unreadable directories
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(dir, e.Name())
		if match == nil || match(child) {
			*results = append(*results, child)
			continue // don't recurse into matched dirs
		}
		if err := walkDirs(child, current+1, max, match, results); err != nil {
			return err
		}
	}
	return nil
}

// ListScanner wraps an explicit list of paths into Items.
type ListScanner struct {
	Paths []string
}

// Scan implements Scanner for ListScanner.
func (s *ListScanner) Scan(_ context.Context) ([]Item, error) {
	items := make([]Item, len(s.Paths))
	for i, p := range s.Paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		items[i] = Item{Name: filepath.Base(abs), Path: abs}
	}
	slices.SortFunc(items, func(a, b Item) int {
		return strings.Compare(a.Name, b.Name)
	})
	return items, nil
}

// GlobScanner finds files/dirs matching a glob pattern.
type GlobScanner struct {
	Pattern string
	Root    string // base directory for relative patterns
}

// Scan implements Scanner for GlobScanner.
func (s *GlobScanner) Scan(_ context.Context) ([]Item, error) {
	pattern := s.Pattern
	if s.Root != "" && !filepath.IsAbs(pattern) {
		pattern = filepath.Join(s.Root, pattern)
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}

	slices.Sort(matches)
	items := make([]Item, len(matches))
	for i, m := range matches {
		abs, err := filepath.Abs(m)
		if err != nil {
			abs = m
		}
		items[i] = Item{Name: filepath.Base(abs), Path: abs}
	}
	return items, nil
}
