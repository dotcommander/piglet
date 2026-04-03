package bulk

import (
	"context"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
)

// Filter returns true for items to keep. Runs in parallel via Apply.
type Filter func(ctx context.Context, item Item) (bool, error)

// ShellFilter returns a Filter that runs a shell command in the item's directory.
// Keeps items where the command exits 0.
func ShellFilter(command string) Filter {
	return func(ctx context.Context, item Item) (bool, error) {
		_, err := shellExec(ctx, item.Path, "", command)
		return err == nil, nil
	}
}

// Apply runs the filter on items with bounded concurrency.
// Returns matched items sorted by name. Errors are silently skipped (item excluded).
func Apply(ctx context.Context, items []Item, f Filter, concurrency int) ([]Item, error) {
	if f == nil {
		return items, nil
	}
	if concurrency <= 0 {
		concurrency = 8
	}

	type candidate struct {
		item  Item
		match bool
	}

	results := make([]candidate, len(items))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i, item := range items {
		g.Go(func() error {
			match, err := f(ctx, item)
			if err != nil {
				return nil // skip errors silently
			}
			results[i] = candidate{item: item, match: match}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	var matched []Item
	for _, r := range results {
		if r.match {
			matched = append(matched, r.item)
		}
	}

	slices.SortFunc(matched, func(a, b Item) int {
		return strings.Compare(a.Name, b.Name)
	})

	return matched, nil
}
