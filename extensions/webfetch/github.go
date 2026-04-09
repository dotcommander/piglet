package webfetch

import (
	"fmt"
	"regexp"
	"strings"
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
