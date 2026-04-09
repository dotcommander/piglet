package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

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
