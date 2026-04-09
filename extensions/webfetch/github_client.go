package webfetch

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitHubClient handles GitHub repo cloning and API fallback.
type GitHubClient struct {
	config     *GitHubConfig
	httpClient *http.Client
}

// NewGitHubClient creates a new GitHub client.
func NewGitHubClient(cfg *GitHubConfig) *GitHubClient {
	if cfg == nil {
		cfg = &GitHubConfig{Enabled: true, SkipLargeRepos: true}
	}
	return &GitHubClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Fetch retrieves content from a GitHub URL.
// Returns nil if the URL is not a GitHub URL (caller should try other providers).
func (g *GitHubClient) Fetch(ctx context.Context, rawURL string) (*GitHubResult, error) {
	parsed, ok := parseGitHubURL(rawURL)
	if !ok {
		return nil, nil // Not a GitHub URL
	}

	// Commit URLs always use API (no clone needed)
	if parsed.IsCommit {
		return g.fetchCommit(ctx, parsed)
	}

	// Clean up old clones before creating new ones
	g.cleanupOldClones()

	// Check repo size if skipLargeRepos is enabled
	if g.config.SkipLargeRepos {
		tooLarge, err := g.checkRepoSize(ctx, parsed)
		if err != nil {
			slog.Debug("failed to check repo size, falling back to API", "error", err)
			return g.fetchViaAPI(ctx, parsed)
		}
		if tooLarge {
			slog.Debug("repo too large, using API", "owner", parsed.Owner, "repo", parsed.Repo)
			return g.fetchViaAPI(ctx, parsed)
		}
	}

	// Try clone first
	result, err := g.clone(ctx, parsed)
	if err != nil {
		slog.Debug("clone failed, falling back to API", "error", err)
		return g.fetchViaAPI(ctx, parsed)
	}

	return result, nil
}

// clone performs a shallow clone and builds the result.
func (g *GitHubClient) clone(ctx context.Context, parsed *githubURL) (*GitHubResult, error) {
	localPath := filepath.Join(os.TempDir(), fmt.Sprintf("piglet-gh-%s-%s", parsed.Owner, parsed.Repo))

	// Check if already cloned
	if _, err := os.Stat(localPath); err == nil {
		// Already exists, use it
		slog.Debug("using existing clone", "path", localPath)
	} else {
		// Clone the repo
		cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", parsed.Owner, parsed.Repo)

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, localPath)
		if parsed.Branch != "" {
			cmd.Args = append(cmd.Args, "--branch", parsed.Branch)
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git clone failed: %w: %s", err, string(output))
		}
	}

	// Build result from cloned repo
	result := &GitHubResult{
		LocalPath: localPath,
		UsedAPI:   false,
	}

	// Read README
	readmePath := filepath.Join(localPath, parsed.Path, "README.md")
	if data, err := os.ReadFile(readmePath); err == nil {
		result.README = string(data)
	}

	// Build tree
	tree, err := g.buildTree(localPath, parsed.Path)
	if err != nil {
		slog.Debug("failed to build tree", "error", err)
	}
	result.Tree = tree

	return result, nil
}

const maxTreeEntries = 10000

// buildTree creates a file tree listing (capped at maxTreeEntries).
func (g *GitHubClient) buildTree(localPath, subPath string) ([]string, error) {
	root := filepath.Join(localPath, subPath)
	var tree []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory
		if strings.Contains(path, "/.git/") || filepath.Base(path) == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		if d.IsDir() {
			tree = append(tree, relPath+"/")
		} else {
			tree = append(tree, relPath)
		}

		if len(tree) >= maxTreeEntries {
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return tree, nil
}

// cleanupOldClones removes clone directories older than 1 hour.
func (g *GitHubClient) cleanupOldClones() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-1 * time.Hour)

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "piglet-gh-") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(os.TempDir(), entry.Name())
			if err := os.RemoveAll(path); err != nil {
				slog.Debug("failed to cleanup old clone", "path", path, "error", err)
			} else {
				slog.Debug("cleaned up old clone", "path", path)
			}
		}
	}
}
