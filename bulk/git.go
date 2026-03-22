package bulk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GitRepoScanner returns a DirScanner configured to find git repositories.
func GitRepoScanner(root string, depth int) *DirScanner {
	return &DirScanner{
		Root:  root,
		Depth: depth,
		Match: func(path string) bool {
			info, err := os.Stat(filepath.Join(path, ".git"))
			return err == nil && info.IsDir()
		},
	}
}

// GitFilter returns a Filter for common git conditions.
// Valid names: all, dirty, clean, ahead, behind, diverged.
func GitFilter(name string) (Filter, error) {
	switch name {
	case "all", "":
		return nil, nil // nil filter = no filtering
	case "dirty":
		return gitStatusFilter(false), nil
	case "clean":
		return gitStatusFilter(true), nil
	case "ahead":
		return gitRevListFilter("@{upstream}..HEAD"), nil
	case "behind":
		return gitRevListFilter("HEAD..@{upstream}"), nil
	case "diverged":
		return gitDivergedFilter(), nil
	default:
		return nil, fmt.Errorf("unknown git filter: %q", name)
	}
}

// gitStatusFilter checks working tree: wantClean=true keeps clean repos, false keeps dirty.
func gitStatusFilter(wantClean bool) Filter {
	return func(ctx context.Context, item Item) (bool, error) {
		out, err := shellExec(ctx, item.Path, "", "git status --porcelain")
		if err != nil {
			return false, err
		}
		isDirty := strings.TrimSpace(out) != ""
		if wantClean {
			return !isDirty, nil
		}
		return isDirty, nil
	}
}

// gitRevListFilter returns a Filter that checks rev-list count > 0.
func gitRevListFilter(rangeSpec string) Filter {
	return func(ctx context.Context, item Item) (bool, error) {
		out, err := shellExec(ctx, item.Path, "", "git rev-list --count "+rangeSpec)
		if err != nil {
			return false, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(out))
		if err != nil {
			return false, err
		}
		return n > 0, nil
	}
}

// gitDivergedFilter returns a Filter matching repos that are both ahead and behind.
func gitDivergedFilter() Filter {
	aheadFilter := gitRevListFilter("@{upstream}..HEAD")
	behindFilter := gitRevListFilter("HEAD..@{upstream}")
	return func(ctx context.Context, item Item) (bool, error) {
		ahead, err := aheadFilter(ctx, item)
		if err != nil || !ahead {
			return false, err
		}
		return behindFilter(ctx, item)
	}
}
