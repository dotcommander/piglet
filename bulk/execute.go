package bulk

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// Run executes the command on each item in parallel with bounded concurrency.
// Returns results sorted by item name.
func Run(ctx context.Context, items []Item, cmd Command, cfg Config) []Result {
	cfg.defaults()

	results := make([]Result, len(items))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Concurrency)

	for i, item := range items {
		g.Go(func() error {
			results[i] = runOne(ctx, item, cmd, cfg.Timeout)
			return nil
		})
	}

	_ = g.Wait()

	slices.SortFunc(results, func(a, b Result) int {
		return strings.Compare(a.Item, b.Item)
	})

	return results
}

// runOne executes a single command on one item with timeout.
func runOne(ctx context.Context, item Item, cmd Command, timeout time.Duration) Result {
	expanded := expandTemplate(cmd.Template, item)

	itemCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := shellExec(itemCtx, item.Path, cmd.Shell, expanded)
	if err != nil {
		return Result{
			Item:   item.Name,
			Path:   item.Path,
			Status: "error",
			Output: err.Error(),
		}
	}

	output := strings.TrimSpace(out)
	if output == "" {
		output = "ok"
	}

	return Result{
		Item:   item.Name,
		Path:   item.Path,
		Status: "ok",
		Output: output,
	}
}

// Execute runs the full collect → filter → execute pipeline.
func Execute(ctx context.Context, scanner Scanner, filter Filter, cmd Command, cfg Config) (Summary, error) {
	cfg.defaults()

	items, err := scanner.Scan(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("scan: %w", err)
	}

	matched, err := Apply(ctx, items, filter, cfg.Concurrency)
	if err != nil {
		return Summary{}, fmt.Errorf("filter: %w", err)
	}

	summary := Summary{
		TotalCollected: len(items),
		MatchedFilter:  len(matched),
	}

	if cfg.DryRun {
		dryResults := make([]Result, len(matched))
		for i, item := range matched {
			dryResults[i] = Result{
				Item:   item.Name,
				Path:   item.Path,
				Status: "skipped",
				Output: fmt.Sprintf("dry run — would run: %s", expandTemplate(cmd.Template, item)),
			}
		}
		summary.Results = dryResults
		summary.Message = fmt.Sprintf(
			"%d items collected, %d matched filter. Dry run — pass dry_run:false to execute.",
			len(items), len(matched),
		)
		return summary, nil
	}

	results := Run(ctx, matched, cmd, cfg)
	summary.Results = results

	ok, failed := 0, 0
	for _, r := range results {
		if r.Status == "ok" {
			ok++
		} else {
			failed++
		}
	}

	if failed == 0 {
		summary.Message = fmt.Sprintf("%d items collected, %d matched, %d succeeded.", len(items), len(matched), ok)
	} else {
		summary.Message = fmt.Sprintf("%d items collected, %d matched, %d succeeded, %d failed.", len(items), len(matched), ok, failed)
	}

	return summary, nil
}
