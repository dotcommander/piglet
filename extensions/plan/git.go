package plan

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitClient handles git operations for checkpoint commits.
type GitClient struct {
	cwd string
}

// NewGitClient creates a git client. Returns nil if not in a git repo.
func NewGitClient(cwd string) *GitClient {
	gitDir := filepath.Join(cwd, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		// Check if we're in a submodule or worktree
		cmd := exec.Command("git", "rev-parse", "--git-dir")
		cmd.Dir = cwd
		if err := cmd.Run(); err != nil {
			return nil
		}
	}
	return &GitClient{cwd: cwd}
}

// IsClean returns true if there are no uncommitted changes.
func (g *GitClient) IsClean() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = g.cwd
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(out) == 0
}

// HasChanges returns true if there are uncommitted changes.
func (g *GitClient) HasChanges() bool {
	return !g.IsClean()
}

// Checkpoint creates a commit with the current changes.
// Returns the commit SHA or an error.
func (g *GitClient) Checkpoint(slug string, stepID int, stepText string) (string, error) {
	// Stage all changes
	stageCmd := exec.Command("git", "add", "-A")
	stageCmd.Dir = g.cwd
	if out, err := stageCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %w: %s", err, string(out))
	}

	// Check if there's anything to commit
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = g.cwd
	out, err := statusCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	if len(out) == 0 {
		// Nothing to commit, return current HEAD
		return g.Head()
	}

	// Truncate step text if too long (rune-safe)
	if runes := []rune(stepText); len(runes) > 72 {
		stepText = string(runes[:69]) + "..."
	}

	// Create commit
	msg := fmt.Sprintf("[plan:%s] step %d: %s", slug, stepID, stepText)
	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Dir = g.cwd
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %w: %s", err, string(out))
	}

	return g.Head()
}

// Head returns the current HEAD commit SHA.
func (g *GitClient) Head() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = g.cwd
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ShortSHA returns the short form of a commit SHA.
func ShortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
