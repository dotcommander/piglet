package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// GitHubConfig configures GitHub clone behavior.
type GitHubConfig struct {
	Enabled        bool `yaml:"enabled"`
	SkipLargeRepos bool `yaml:"skip_large_repos"`
}

// GitHubResult holds the result of fetching a GitHub repo.
type GitHubResult struct {
	LocalPath string   `json:"local_path"`
	README    string   `json:"readme"`
	Tree      []string `json:"tree"`
	UsedAPI   bool     `json:"used_api"`
}

// githubURL represents a parsed GitHub URL.
type githubURL struct {
	Owner    string
	Repo     string
	Branch   string // empty means default branch
	Path     string // path within repo (empty means root)
	Commit   string // full commit SHA (for commit URLs)
	IsCommit bool   // true if URL points to a specific commit
}

// parseGitHubURL parses various GitHub URL formats.
// Supported formats:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo/tree/branch
//   - https://github.com/owner/repo/tree/branch/path
//   - https://github.com/owner/repo/commit/sha
func parseGitHubURL(rawURL string) (*githubURL, bool) {
	// Normalize URL
	rawURL = strings.TrimSpace(rawURL)

	// Match GitHub URLs
	re := regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)(?:/(tree/([^/]+)(?:/(.*))?|commit/([a-f0-9]{40}))?)/?$`)
	matches := re.FindStringSubmatch(rawURL)
	if matches == nil {
		return nil, false
	}

	result := &githubURL{
		Owner: matches[1],
		Repo:  strings.TrimSuffix(matches[2], ".git"),
	}

	// Check if it's a commit URL
	if matches[6] != "" {
		result.Commit = matches[6]
		result.IsCommit = true
		return result, true
	}

	// Tree URL or root repo URL
	result.Branch = matches[4]
	result.Path = matches[5]

	return result, true
}

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

// checkRepoSize checks if the repo exceeds the size limit.
// Returns (true, nil) if repo is too large, (false, nil) if OK.
func (g *GitHubClient) checkRepoSize(ctx context.Context, parsed *githubURL) (bool, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", parsed.Owner, parsed.Repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Repo not found via API, might be private - try clone anyway
		return false, nil
	}

	if resp.StatusCode >= 400 {
		return false, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	var repoInfo struct {
		Size int `json:"size"` // in KB
	}

	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return false, err
	}

	// 350MB threshold (size is in KB)
	const maxSizeKB = 350 * 1024
	return repoInfo.Size > maxSizeKB, nil
}

// fetchViaAPI fetches repo content via GitHub API.
func (g *GitHubClient) fetchViaAPI(ctx context.Context, parsed *githubURL) (*GitHubResult, error) {
	result := &GitHubResult{
		UsedAPI: true,
	}

	// Fetch README
	readmeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/readme", parsed.Owner, parsed.Repo)
	if parsed.Branch != "" {
		readmeURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/readme?ref=%s", parsed.Owner, parsed.Repo, parsed.Branch)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readmeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create readme request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3.raw")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch readme: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
		result.README = string(data)
	}

	// Fetch tree
	treeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/HEAD?recursive=1", parsed.Owner, parsed.Repo)
	if parsed.Branch != "" {
		treeURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", parsed.Owner, parsed.Repo, parsed.Branch)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, treeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create tree request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err = g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch tree: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var treeResp struct {
			Tree []struct {
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"tree"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&treeResp); err == nil {
			for _, item := range treeResp.Tree {
				if item.Type == "tree" {
					result.Tree = append(result.Tree, item.Path+"/")
				} else {
					result.Tree = append(result.Tree, item.Path)
				}
			}
		}
	}

	return result, nil
}

// fetchCommit fetches commit info via API (no clone for commit URLs).
func (g *GitHubClient) fetchCommit(ctx context.Context, parsed *githubURL) (*GitHubResult, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", parsed.Owner, parsed.Repo, parsed.Commit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create commit request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch commit: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	var commitInfo struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name  string    `json:"name"`
				Date  time.Time `json:"date"`
				Email string    `json:"email"`
			} `json:"author"`
		} `json:"commit"`
		HTMLURL string `json:"html_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&commitInfo); err != nil {
		return nil, fmt.Errorf("decode commit: %w", err)
	}

	result := &GitHubResult{
		UsedAPI: true,
		Tree:    []string{commitInfo.SHA},
		README: fmt.Sprintf("# Commit %s\n\n**Author:** %s <%s>\n**Date:** %s\n\n%s\n\n[View on GitHub](%s)",
			commitInfo.SHA[:7],
			commitInfo.Commit.Author.Name,
			commitInfo.Commit.Author.Email,
			commitInfo.Commit.Author.Date.Format(time.RFC3339),
			commitInfo.Commit.Message,
			commitInfo.HTMLURL,
		),
	}

	return result, nil
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

// FormatGitHubResult renders a GitHubResult as markdown.
func FormatGitHubResult(result *GitHubResult) string {
	var b strings.Builder

	if result.UsedAPI {
		b.WriteString("> **Note:** Used GitHub API (repo too large or commit URL)\n\n")
	}

	if result.README != "" {
		b.WriteString("## README\n\n")
		b.WriteString(result.README)
		b.WriteString("\n\n")
	}

	if len(result.Tree) > 0 {
		b.WriteString("## File Tree\n\n```\n")
		for _, path := range result.Tree {
			b.WriteString(path)
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	if result.LocalPath != "" {
		b.WriteString(fmt.Sprintf("\n*Cloned to: %s*\n", result.LocalPath))
	}

	return b.String()
}
